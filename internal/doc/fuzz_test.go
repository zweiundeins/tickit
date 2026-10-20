package doc

import (
	"encoding/json"
	"fmt"
	"testing"
)

type fuzzCaseJSON struct {
	Doc     json.RawMessage `json:"doc"`
	Commits []commitJSON    `json:"commits"`
	Final   json.RawMessage `json:"final"`
}

// runFuzzCase has the oracle's generator make a document and commits made
// on versions up to maxK behind the head, rebased by applyCommit in goja;
// Go rebases the same commits and must apply the same steps (FR-82.4 and
// MP-05: the browser and the writer drop the same steps).
func runFuzzCase(t *testing.T, seed uint32, kb, commits, maxK, edits int) (int, int) {
	o := theOracle(t)
	opts := fmt.Sprintf(`{"kb":%d,"commits":%d,"maxK":%d,"edits":%d}`, kb, commits, maxK, edits)
	raw := o.must(t, "fuzzCase", seed, opts)
	var c fuzzCaseJSON
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	d, err := ParseDoc(c.Doc)
	if err != nil {
		t.Fatalf("seed %d: %v", seed, err)
	}
	var history [][]Step
	steps, dropped := 0, 0
	for ci, cm := range c.Commits {
		st, err := ParseSteps(cm.Steps)
		if err != nil {
			t.Fatalf("seed %d commit %d: %v", seed, ci, err)
		}
		var since []Step
		for v := cm.Base; v < len(history); v++ {
			since = append(since, history[v]...)
		}
		next, applied, err := Rebase(d, since, st)
		if cm.Error != nil {
			if err == nil {
				t.Fatalf("seed %d commit %d: the oracle throws %q, Go applies it", seed, ci, *cm.Error)
			}
			continue
		}
		if err != nil {
			t.Fatalf("seed %d commit %d (base %d of %d): Go: %v\nsteps %s", seed, ci, cm.Base, len(history), err, cm.Steps)
		}
		got, _ := MarshalSteps(applied)
		if string(got) != string(cm.Applied) {
			t.Fatalf("seed %d commit %d (base %d of %d): applied steps differ\nsteps %s\n go %s\n js %s", seed, ci, cm.Base, len(history), cm.Steps, got, cm.Applied)
		}
		steps += len(st)
		dropped += len(st) - len(applied)
		d = next
		history = append(history, applied)
	}
	got, _ := MarshalDoc(d)
	if string(got) != string(c.Final) {
		t.Fatalf("seed %d: final documents differ at byte %d", seed, firstDiff(string(c.Final), string(got)))
	}
	return steps, dropped
}

func TestRebaseGenerated(t *testing.T) {
	n := 60
	if testing.Short() {
		n = 10
	}
	steps, dropped := 0, 0
	for seed := uint32(1); seed <= uint32(n); seed++ {
		s, d := runFuzzCase(t, seed, 2, 12, int(seed%10), 4)
		steps += s
		dropped += d
	}
	t.Logf("%d cases, %d steps, %d dropped by both", n, steps, dropped)
}

func FuzzRebase(f *testing.F) {
	for _, s := range []uint32{1, 2, 3, 42} {
		f.Add(s, uint8(5), uint8(4))
	}
	f.Fuzz(func(t *testing.T, seed uint32, k, edits uint8) {
		runFuzzCase(t, seed, 1+int(seed%3), 10, int(k%16), 1+int(edits%6))
	})
}
