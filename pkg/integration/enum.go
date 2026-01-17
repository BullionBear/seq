package integration

type Exchange int

const (
	ExchangeBinance Exchange = iota
	ExchangeBybit
)

type ProductType int

const (
	ProductTypeSpot ProductType = iota
	ProductTypePerpetual
)

type Side int

const (
	SideBuy Side = iota
	SideSell
)

type OrderType int

const (
	OrderTypeLimit OrderType = iota
	OrderTypeMarket
)

type TimeInForce int

const (
	TimeInForceGTC TimeInForce = iota
	TimeInForceIOC
	TimeInForceFOK
	TimeInForcePO
)
