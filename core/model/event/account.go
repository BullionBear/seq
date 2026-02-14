package event

type Balance struct {
	TokenID   int
	Available float64
	Locked    float64
	Total     float64
}

type BalanceUpdate struct {
	AccountID int
	Balances  []Balance
	UpdatedAt uint64
}

func (b BalanceUpdate) Topic() Topic {
	return TopicEventBalanceUpdate
}

type RespBalanceSnapshot struct {
	AccountID int
	Balances  []Balance
}

func (r RespBalanceSnapshot) Topic() Topic {
	return TopicEventRespBalanceSnapshot
}
