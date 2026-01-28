package adapter

type Exchange int

const (
	ExchangeUnknown Exchange = iota
	ExchangeBinance
	ExchangeBybit
)

type ProductType int

const (
	ProductTypeUnknown ProductType = iota
	ProductTypeSpot
	ProductTypePerpetual
)
