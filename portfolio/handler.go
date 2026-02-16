package portfolio

import "github.com/BullionBear/seq/core/model/event"

// BalanceEngineHandler is implemented by the portfolio engine.
// It receives event callbacks and lifecycle notifications from BalanceActor.
type BalanceEngineHandler interface {
	OnBalanceUpdate(ev event.BalanceUpdate)
	OnRespBalanceSnapshot(ev event.RespBalanceSnapshot)
	OnFill(ev event.Fill)
	NotifyReady()
}
