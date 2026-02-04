package common

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

type OrderStatus int

const (
	OrderStatusUninitialized OrderStatus = iota
	OrderStatusInitialized
	OrderStatusInFlight
	OrderStatusAccepted
	OrderStatusPartiallyFilled
	OrderStatusFilled
	OrderStatusCanceled
	OrderStatusRejected
	OrderStatusUnknown
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

type EngineType int

const (
	EngineData EngineType = iota
	EngineExecution
	EnginePortfolio
	EngineRisk
	EngineStrategy
)
