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

type WalletType int

const (
	WalletTypeUnknown  WalletType = iota
	WalletTypeSpot                // id=1
	WalletTypeUMargin             // id=2 (USD Margin)
	WalletTypeCMargin             // id=3 (Coin Margin)
	WalletTypeLeverage            // id=4
	WalletTypeUnified             // id=5
)
