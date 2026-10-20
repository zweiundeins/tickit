package doc

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/dop251/goja"
)

// The oracle: testdata/oracle/oracle.js (prosemirror-model 1.25.12 and
// prosemirror-transform 1.12.2 with the tickit schema) run in goja. Only
// tests import goja.

// goja's String.prototype.split leaves empty strings between the tokens of
// prosemirror-model's content-expression tokenizer, which V8 does not; the
// patch drops them, which V8 never produces in the middle.
const (
	splitCall    = `.split(/\s*(?=\b|\W|$)/)`
	splitPatched = `.split(/\s*(?=\b|\W|$)/).filter(function(t){return t!==""})`
)

var (
	oracleOnce sync.Once
	oracleProg *goja.Program
	oracleErr  error
)

type oracle struct {
	mu sync.Mutex
	vm *goja.Runtime
	o  *goja.Object
}

func loadOracle(t testing.TB) *oracle {
	t.Helper()
	oracleOnce.Do(func() {
		src, err := os.ReadFile("testdata/oracle/oracle.js")
		if err != nil {
			oracleErr = err
			return
		}
		s := string(src)
		if !strings.Contains(s, splitCall) {
			oracleErr = errString("oracle.js: the tokenizer's split call is gone; update the patch")
			return
		}
		s = strings.Replace(s, splitCall, splitPatched, 1)
		oracleProg, oracleErr = goja.Compile("oracle.js", s, false)
	})
	if oracleErr != nil {
		t.Fatal(oracleErr)
	}
	vm := goja.New()
	if _, err := vm.RunProgram(oracleProg); err != nil {
		t.Fatal(err)
	}
	o := &oracle{vm: vm, o: vm.Get("oracle").ToObject(vm)}
	if _, err := o.call("init", string(schemaJSON)); err != nil {
		t.Fatal(err)
	}
	return o
}

type errString string

func (e errString) Error() string { return string(e) }

var (
	sharedOracleOnce sync.Once
	sharedOracle     *oracle
)

// theOracle returns one oracle shared by the tests of the process.
func theOracle(t testing.TB) *oracle {
	sharedOracleOnce.Do(func() { sharedOracle = loadOracle(t) })
	if sharedOracle == nil {
		t.Fatal("oracle failed to load")
	}
	return sharedOracle
}

var (
	fuzzOracleMu   sync.Mutex
	fuzzOracle     *oracle
	fuzzOracleUses int
)

// fuzzRuntime returns an oracle that is replaced every 100 uses: goja keeps
// what prosemirror-model's WeakMap caches hold, and a fuzz worker would grow
// past a gigabyte in a few thousand cases.
func fuzzRuntime(t testing.TB) *oracle {
	fuzzOracleMu.Lock()
	defer fuzzOracleMu.Unlock()
	if fuzzOracle == nil || fuzzOracleUses >= 100 {
		fuzzOracle, fuzzOracleUses = loadOracle(t), 0
	}
	fuzzOracleUses++
	return fuzzOracle
}

// call runs oracle[name](args...) and returns the result as a string; a
// JavaScript exception is the error.
func (o *oracle) call(name string, args ...any) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	fn, ok := goja.AssertFunction(o.o.Get(name))
	if !ok {
		return "", errString("oracle has no function " + name)
	}
	vs := make([]goja.Value, len(args))
	for i, a := range args {
		vs[i] = o.vm.ToValue(a)
	}
	v, err := fn(goja.Undefined(), vs...)
	if err != nil {
		if ex, ok := err.(*goja.Exception); ok {
			return "", errString(ex.Value().String())
		}
		return "", err
	}
	return v.String(), nil
}

func (o *oracle) must(t testing.TB, name string, args ...any) string {
	t.Helper()
	s, err := o.call(name, args...)
	if err != nil {
		t.Fatalf("oracle.%s: %v", name, err)
	}
	return s
}

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// rawList splits a JSON array into its elements.
func rawList(t testing.TB, data []byte) []json.RawMessage {
	t.Helper()
	var out []json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
