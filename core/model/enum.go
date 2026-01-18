package model

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

type Status int

const (
	StatusUninitialized Status = iota
	StatusInitialized
	StatusInFlight
	StatusAccepted
	StatusPartiallyFilled
	StatusFilled
	StatusCanceled
	StatusRejected
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
