package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// server is `tickit serve` in a process of its own, with the unit's
// memory settings in its environment, pinned to its own cores.
type server struct {
	bin, db, addr, metrics, cpus, logPath string

	cmd     *exec.Cmd
	exited  chan struct{}
	mu      sync.Mutex
	maxRSS  int64 // bytes, of the samples
	maxAnon int64 // RssAnon: the heap, SQLite's caches; what MemoryMax cannot reclaim
	maxFile int64 // RssFile: the database's pages mapped by the readers (mmap), reclaimable
}

func (s *server) url() string { return "http://" + s.addr }

// start runs the binary and waits until /healthz answers; it returns that
// moment.
func (s *server) start() (time.Time, error) {
	if res, err := http.Get(s.url() + "/healthz"); err == nil {
		res.Body.Close()
		return time.Time{}, fmt.Errorf("something serves %s already", s.addr)
	}
	args := []string{s.bin, "serve", "-db", s.db, "-addr", s.addr, "-metrics", s.metrics, "-streams", "200"}
	if s.cpus != "" {
		args = append([]string{"taskset", "-c", s.cpus}, args...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "GOMEMLIMIT=448MiB")
	f, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return time.Time{}, err
	}
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		f.Close()
		return time.Time{}, err
	}
	s.cmd, s.exited = cmd, make(chan struct{})
	go func() { cmd.Wait(); f.Close(); close(s.exited) }()
	go s.sample(cmd.Process.Pid, s.exited)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(s.url() + "/healthz")
		if err == nil {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return time.Now(), nil
			}
		}
		select {
		case <-s.exited:
			return time.Time{}, fmt.Errorf("the server exited; see %s", s.logPath)
		case <-time.After(20 * time.Millisecond):
		}
	}
	return time.Time{}, fmt.Errorf("no /healthz within 60 s")
}

// stop sends SIGTERM, as systemd does, and waits for the exit.
func (s *server) stop() time.Duration {
	start := time.Now()
	s.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-s.exited:
	case <-time.After(40 * time.Second):
		s.cmd.Process.Kill()
		<-s.exited
	}
	return time.Since(start)
}

// sample reads the process's resident memory every 100 ms.
func (s *server) sample(pid int, done chan struct{}) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		if rss := procStatus(pid, "VmRSS"); rss > 0 {
			anon, file := procStatus(pid, "RssAnon"), procStatus(pid, "RssFile")
			s.mu.Lock()
			s.maxRSS = max(s.maxRSS, rss)
			s.maxAnon = max(s.maxAnon, anon)
			s.maxFile = max(s.maxFile, file)
			s.mu.Unlock()
		}
	}
}

// procStatus reads a field of /proc/<pid>/status in bytes.
func procStatus(pid int, field string) int64 {
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), field+":"); ok {
			kb, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "kB")), 10, 64)
			return kb << 10
		}
	}
	return 0
}

// cpu is the process's user and system time.
func (s *server) cpu() time.Duration {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", s.cmd.Process.Pid))
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b[strings.LastIndexByte(string(b), ')')+2:]))
	ut, _ := strconv.ParseInt(f[11], 10, 64)
	st, _ := strconv.ParseInt(f[12], 10, 64)
	return time.Duration(ut+st) * time.Second / 100 // USER_HZ
}

func (s *server) rss() (now, peak int64) {
	pid := s.cmd.Process.Pid
	return procStatus(pid, "VmRSS"), procStatus(pid, "VmHWM")
}

// metrics are the server's Prometheus numbers, by name and labels.
type metrics map[string]float64

func (s *server) scrape(ctx context.Context) (metrics, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+s.metrics+"/metrics", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	m := metrics{}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		if i < 0 {
			continue
		}
		v, err := strconv.ParseFloat(line[i+1:], 64)
		if err == nil {
			m[line[:i]] = v
		}
	}
	return m, sc.Err()
}

// sum adds every series of a metric whatever its labels.
func (m metrics) sum(name string) float64 {
	var t float64
	for k, v := range m {
		if k == name || strings.HasPrefix(k, name+"{") {
			t += v
		}
	}
	return t
}

// overruns are the server log's tick overrun reports.
func overrunLines(path string) []string {
	b, _ := os.ReadFile(path)
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "tick overrun") || strings.Contains(l, "render overrun") {
			out = append(out, l)
		}
	}
	return out
}

// cpuProfile takes the server's CPU profile for d (pprof.go).
func (s *server) cpuProfile(ctx context.Context, d time.Duration, path string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/debug/pprof/profile?seconds=%d", s.metrics, int(d.Seconds())), nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("pprof: %d (build the binary with -tags pprof)", res.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, res.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// selfCPU is the load tool's own user and system time.
func selfCPU() time.Duration {
	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}
