package event

import "github.com/BullionBear/seq/core/model/common"

type BalanceUpdate struct {
	AccountID int
	Balances  []common.Balance
	UpdatedAt uint64
}

func (b BalanceUpdate) Topic() Topic {
	return TopicEventBalanceUpdate
}

type RespBalanceSnapshot struct {
	AccountID int
	Balances  []common.Balance
}

func (r RespBalanceSnapshot) Topic() Topic {
	return TopicEventRespBalanceSnapshot
}
