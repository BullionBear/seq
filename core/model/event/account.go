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

type BalanceSnapshot struct {
	AccountID int
	Balances  []Balance
}
