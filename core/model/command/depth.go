package command

type ReqDepthSnapshot struct {
	SymbolID int
}

func (r ReqDepthSnapshot) CommandType() CommandType {
	return CommandTypeReqDepthSnapshot
}
