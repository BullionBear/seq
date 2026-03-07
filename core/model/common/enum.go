package common

type Exchange int

const (
	ExchangeUnknown Exchange = iota
	ExchangeBinance          // id=1
	ExchangeBybit            // id=2
)

type ProductType int

const (
	ProductTypeUnknown   ProductType = iota
	ProductTypeSpot                  // id=1
	ProductTypePerpetual             // id=2
)

type WalletType int

const (
	WalletTypeUnknown  WalletType = iota
	WalletTypeSpot                // id=1
	WalletTypeUMargin             // id=2 (USD Margin)
	WalletTypeCMargin             // id=3 (Coin Margin)
	WalletTypeLeverage            // id=4
	WalletTypeUnified             // id=5
)

type OrderStatus int

const (
	OrderStatusUninitialized OrderStatus = iota
	OrderStatusInitialized
	OrderStatusAccepted
	OrderStatusPartiallyFilled
	OrderStatusFilled
	OrderStatusCanceled
	OrderStatusRejected
	OrderStatusUnknown
)

func (o OrderStatus) Cancellable() bool {
	return o == OrderStatusAccepted || o == OrderStatusPartiallyFilled
}

func (o OrderStatus) IsOpen() bool {
	return o == OrderStatusAccepted || o == OrderStatusPartiallyFilled || o == OrderStatusInitialized
}

func (o OrderStatus) IsTerminal() bool {
	return o == OrderStatusFilled || o == OrderStatusCanceled || o == OrderStatusRejected
}

type Side int

const (
	SideUnknown Side = iota
	SideBuy
	SideSell
)

func (s Side) String() string {
	switch s {
	case SideBuy:
		return "Buy"
	case SideSell:
		return "Sell"
	default:
		return "Unknown"
	}
}

type OrderType int

const (
	OrderTypeLimit OrderType = iota
	OrderTypeMarket
)

func (o OrderType) String() string {
	switch o {
	case OrderTypeLimit:
		return "Limit"
	case OrderTypeMarket:
		return "Market"
	default:
		return "Unknown"
	}
}

type TimeInForce int

const (
	TimeInForceGTC TimeInForce = iota
	TimeInForceIOC
	TimeInForceFOK
	TimeInForcePO
)

func (t TimeInForce) String() string {
	switch t {
	case TimeInForceGTC:
		return "GTC"
	case TimeInForceIOC:
		return "IOC"
	case TimeInForceFOK:
		return "FOK"
	case TimeInForcePO:
		return "PO"
	default:
		return "Unknown"
	}
}

type EngineType int

const (
	EngineData EngineType = iota
	EngineExecution
	EnginePortfolio
	EngineRisk
	EngineStrategy
)

func (e EngineType) String() string {
	switch e {
	case EngineData:
		return "Data"
	case EngineExecution:
		return "Execution"
	case EnginePortfolio:
		return "Portfolio"
	case EngineRisk:
		return "Risk"
	case EngineStrategy:
		return "Strategy"
	default:
		return "Unknown"
	}
}

type BookState int

const (
	BookStateWaitForSnapshot BookState = iota
	BookStateUpdating
	BookStateReady
)

func (s BookState) String() string {
	switch s {
	case BookStateWaitForSnapshot:
		return "WaitForSnapshot"
	case BookStateUpdating:
		return "Updating"
	case BookStateReady:
		return "Ready"
	default:
		return "Unknown"
	}
}
