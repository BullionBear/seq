package pms

import "github.com/shopspring/decimal"

type Instrument struct {
	SymbolID      int
	Venue         string
	Symbol        string
	BaseCcy       string
	QuoteCcy      string
	PriceTickSize decimal.Decimal
	QtyTickSize   decimal.Decimal
}
