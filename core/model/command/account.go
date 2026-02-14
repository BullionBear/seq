package command

type QryBalanceSnapshot struct {
	AccountID int
}

func (q QryBalanceSnapshot) CommandType() CommandType {
	return CommandTypeQryBalanceSnapshot
}
