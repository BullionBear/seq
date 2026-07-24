package msgbus

import (
	"fmt"

	"github.com/BullionBear/seq/core/model/common"
)

// Phase is the dispatch phase of an event consumer.
//
// EventBus delivers each event to consumers in registration order. Correctness
// depends on cache writers running before cache readers for the same event:
// orderbook must update the book before a strategy prices off it. Phases encode
// that ordering; AssertOrder enforces that they are non-decreasing.
//
// See docs/CONSUMER_ORDER.md.
type Phase int

const (
	PhaseIngest  Phase = iota // data: writes orderbook / kline
	PhaseOrder                // execution: writes order cache
	PhaseAccount              // ledger: writes balance / position
	PhaseControl              // risk
	PhaseDecide               // strategy: read-only
)

func (p Phase) String() string {
	switch p {
	case PhaseIngest:
		return "ingest"
	case PhaseOrder:
		return "order"
	case PhaseAccount:
		return "account"
	case PhaseControl:
		return "control"
	case PhaseDecide:
		return "decide"
	default:
		return fmt.Sprintf("phase(%d)", int(p))
	}
}

// phaseTable is the single source of truth for consumer ordering policy.
// Adding an engine requires adding a row here; there is no default.
var phaseTable = map[common.EngineType]Phase{
	common.EngineData:      PhaseIngest,
	common.EngineExecution: PhaseOrder,
	common.EngineLedger:    PhaseAccount,
	common.EngineRisk:      PhaseControl,
	common.EngineStrategy:  PhaseDecide,
}

// PhaseOf returns the dispatch phase for an engine type.
//
// It panics on an unknown engine type. This is a programming error (a new
// engine was added without a phaseTable row) and it surfaces during Init, never
// on the dispatch hot path. Failing loudly at startup is preferable to silently
// assigning a phase and corrupting dispatch order.
func PhaseOf(t common.EngineType) Phase {
	p, ok := phaseTable[t]
	if !ok {
		panic(fmt.Sprintf(
			"msgbus: no dispatch phase for engine type %s (%d); "+
				"add a row to phaseTable, see docs/CONSUMER_ORDER.md",
			t, int(t)))
	}
	return p
}
