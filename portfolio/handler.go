package portfolio

import (
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// BalanceEngineHandler is implemented by the portfolio engine.
// It receives event callbacks and lifecycle notifications from BalanceActor.
type BalanceEngineHandler interface {
	OnBalanceUpdate(ev event.BalanceUpdate)
	OnRespBalanceSnapshot(ev event.RespBalanceSnapshot)
	OnExecution(ev event.Execution)
	NotifyReady()

	// ResolveWallet resolves a wallet name to its identifiers.
	ResolveWallet(name string) (accountID int, walletID int, walletType common.WalletType, err error)
}
