package doc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

var docFixtures = []string{
	"f20k-doc", "f20k-final", "f200k-doc", "f200k-final",
	"f20k-k0-final", "f20k-k1-final", "f20k-k5-final", "f20k-k20-final", "f200k-k5-final",
}

// Every fixture document parses and writes back byte-equal, as the oracle
// writes it (DM-05: documents are stored as prosemirror-model's JSON).
func TestFixtureDocsRoundTrip(t *testing.T) {
	o := theOracle(t)
	for _, name := range docFixtures {
		data := bytes.TrimSpace(readFixture(t, name+".json"))
		d, err := ParseDoc(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out, _ := MarshalDoc(d)
		if !bytes.Equal(out, data) {
			t.Errorf("%s: Go writes it back differently (%d vs %d bytes)", name, len(out), len(data))
		}
		if want := o.must(t, "roundtrip", string(data)); want != string(out) {
			t.Errorf("%s: the oracle writes it differently", name)
		}
		if msg := o.must(t, "check", string(data)); msg != "" {
			t.Errorf("%s: the oracle refuses it: %s", name, msg)
		}
		if got := d.JSONLen(); got != len(out) {
			t.Errorf("%s: JSONLen %d, want %d", name, got, len(out))
		}
	}
}

// applyAndCompare applies each batch in Go and in the oracle and compares
// every step's outcome, and the documents every `every` batches.
func applyAndCompare(t *testing.T, prefix string, every int) {
	o := theOracle(t)
	data := readFixture(t, prefix+"-doc.json")
	d, err := ParseDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := strconv.Atoi(o.must(t, "load", string(data)))
	defer o.call("free", id)
	steps := 0
	for bi, raw := range rawList(t, readFixture(t, prefix+"-batches.json")) {
		for si, sraw := range rawList(t, raw) {
			st, err := tickit.ParseStep(sraw)
			if err != nil {
				t.Fatalf("batch %d step %d: %v", bi, si, err)
			}
			if got := string(MarshalStep(st)); got != string(sraw) {
				t.Fatalf("batch %d step %d: writes back as %s, want %s", bi, si, got, sraw)
			}
			res, err := o.call("apply", id, string(sraw))
			next, gerr := st.Apply(d)
			switch {
			case err != nil:
				if gerr == nil || IsStepFailed(gerr) {
					t.Fatalf("batch %d step %d: oracle throws %v, Go: %v", bi, si, err, gerr)
				}
			case res != "":
				if !IsStepFailed(gerr) {
					t.Fatalf("batch %d step %d: oracle %s, Go: %v", bi, si, res, gerr)
				}
			default:
				if gerr != nil {
					t.Fatalf("batch %d step %d %s: oracle applies it, Go: %v", bi, si, sraw, gerr)
				}
				d = next
			}
			steps++
		}
		if (bi+1)%every == 0 {
			got, _ := MarshalDoc(d)
			if want := o.must(t, "json", id); want != string(got) {
				t.Fatalf("after batch %d: documents differ at byte %d", bi, firstDiff(want, string(got)))
			}
		}
	}
	got, _ := MarshalDoc(d)
	if want := bytes.TrimSpace(readFixture(t, prefix+"-final.json")); !bytes.Equal(got, want) {
		t.Fatalf("final document differs from V8's at byte %d", firstDiff(string(want), string(got)))
	}
	if err := d.Check(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d steps agree", prefix, steps)
}

func firstDiff(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

func TestBatchesF20k(t *testing.T)  { applyAndCompare(t, "f20k", 1) }
func TestBatchesF200k(t *testing.T) { applyAndCompare(t, "f200k", 20) }

type commitJSON struct {
	Base    int             `json:"base"`
	Steps   json.RawMessage `json:"steps"`
	Applied json.RawMessage `json:"applied"`
	Error   *string         `json:"error"`
}

// replayCommits rebases every commit in Go as the writer does and compares
// the steps as applied with the oracle's, and the final document with V8's.
func replayCommits(t *testing.T, prefix string, k int) {
	o := theOracle(t)
	data := readFixture(t, prefix+"-doc.json")
	d, err := ParseDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := strconv.Atoi(o.must(t, "load", string(data)))
	defer o.call("free", id)
	var commits []commitJSON
	if err := json.Unmarshal(readFixture(t, fmt.Sprintf("%s-k%d-commits.json", prefix, k)), &commits); err != nil {
		t.Fatal(err)
	}
	var history [][]Step
	var historyJSON []string
	nsteps := 0
	for ci, c := range commits {
		steps, err := ParseSteps(c.Steps)
		if err != nil {
			t.Fatalf("commit %d: %v", ci, err)
		}
		nsteps += len(steps)
		var since []Step
		var sinceJSON []string
		for v := c.Base; v < len(history); v++ {
			since = append(since, history[v]...)
			if historyJSON[v] != "[]" {
				sinceJSON = append(sinceJSON, strings.TrimSuffix(strings.TrimPrefix(historyJSON[v], "["), "]"))
			}
		}
		want := o.must(t, "rebase", id, "["+strings.Join(sinceJSON, ",")+"]", string(c.Steps))
		next, applied, err := Rebase(d, since, steps)
		if err != nil {
			t.Fatalf("commit %d: %v", ci, err)
		}
		got, _ := MarshalSteps(applied)
		if string(got) != want {
			t.Fatalf("commit %d (base %d): applied steps differ\n go: %s\njs: %s", ci, c.Base, got, want)
		}
		d = next
		history = append(history, applied)
		historyJSON = append(historyJSON, want)
	}
	got, _ := MarshalDoc(d)
	if want := bytes.TrimSpace(readFixture(t, fmt.Sprintf("%s-k%d-final.json", prefix, k))); !bytes.Equal(got, want) {
		t.Fatalf("final document differs from V8's at byte %d", firstDiff(string(want), string(got)))
	}
	t.Logf("%s k=%d: %d commits, %d steps agree", prefix, k, len(commits), nsteps)
}

func TestRebaseF20kK0(t *testing.T)  { replayCommits(t, "f20k", 0) }
func TestRebaseF20kK1(t *testing.T)  { replayCommits(t, "f20k", 1) }
func TestRebaseF20kK5(t *testing.T)  { replayCommits(t, "f20k", 5) }
func TestRebaseF20kK20(t *testing.T) { replayCommits(t, "f20k", 20) }
func TestRebaseF200kK5(t *testing.T) { replayCommits(t, "f200k", 5) }
