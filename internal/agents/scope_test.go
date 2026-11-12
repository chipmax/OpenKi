package agents

import (
	"testing"

	"github.com/chipmax/loimo/internal/store"
)

func TestResyncDoesNotAdoptFileOnlyProvider(t *testing.T) {
	t.Setenv("LOIMO_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	a := &countingAgent{}
	r := NewRegistry(st)
	r.agents = []Agent{a}
	// The adapter reports itself enabled from the file, but the config does
	// not own it. A second instance must leave that file alone.
	if n := r.ResyncEnabled(); n != 0 {
		t.Fatalf("resync wrote %d adapters without config ownership", n)
	}
	if a.enableCalls.Load() != 0 {
		t.Fatalf("adapter written %d times, want 0", a.enableCalls.Load())
	}
}

func TestScopeNoneBlocksEveryAdapter(t *testing.T) {
	t.Setenv("LOIMO_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	a := &countingAgent{}
	r := NewRegistry(st)
	r.agents = []Agent{a}
	r.SetScopeOverride("none")
	if err := r.Enable("count"); err == nil {
		t.Fatal("Enable must fail outside the process scope")
	}
	if n := r.ResyncEnabled(); n != 0 {
		t.Fatalf("scope none resynced %d adapters", n)
	}
	if a.enableCalls.Load() != 0 {
		t.Fatalf("scope none wrote %d times", a.enableCalls.Load())
	}
}
