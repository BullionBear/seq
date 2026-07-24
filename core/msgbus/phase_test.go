package msgbus

import (
	"testing"

	"github.com/BullionBear/seq/core/model/common"
)

// allEngineTypes must be kept in sync with common.EngineType.
// Go cannot enumerate constants, so this list is the manual guard.
var allEngineTypes = []common.EngineType{
	common.EngineData,
	common.EngineExecution,
	common.EngineLedger,
	common.EngineRisk,
	common.EngineStrategy,
}

func TestPhaseOf_CoversAllEngineTypes(t *testing.T) {
	for _, et := range allEngineTypes {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("PhaseOf(%s) panicked: %v", et, r)
				}
			}()
			_ = PhaseOf(et)
		}()
	}
	if len(phaseTable) != len(allEngineTypes) {
		t.Fatalf("phaseTable has %d rows, allEngineTypes has %d; "+
			"a new EngineType was added without updating one of them",
			len(phaseTable), len(allEngineTypes))
	}
}

// The dispatch contract requires writers before readers. Encode the intended
// relative order so a reshuffle of the constants fails here.
func TestPhaseOrdering(t *testing.T) {
	want := []common.EngineType{
		common.EngineData,      // writes market data
		common.EngineExecution, // writes orders
		common.EngineLedger,    // writes balances
		common.EngineRisk,
		common.EngineStrategy, // reads everything
	}
	for i := 1; i < len(want); i++ {
		prev, cur := PhaseOf(want[i-1]), PhaseOf(want[i])
		if prev >= cur {
			t.Fatalf("PhaseOf(%s)=%s must be strictly less than PhaseOf(%s)=%s",
				want[i-1], prev, want[i], cur)
		}
	}
}

func TestPhaseOf_UnknownPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("PhaseOf should panic on unknown engine type")
		}
	}()
	_ = PhaseOf(common.EngineType(9999))
}

func TestPhaseString(t *testing.T) {
	cases := map[Phase]string{
		PhaseIngest:  "ingest",
		PhaseOrder:   "order",
		PhaseAccount: "account",
		PhaseControl: "control",
		PhaseDecide:  "decide",
		Phase(42):    "phase(42)",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Errorf("Phase(%d).String() = %q, want %q", int(p), got, want)
		}
	}
}
