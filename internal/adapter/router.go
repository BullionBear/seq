package adapter

import (
	"fmt"

	"github.com/BullionBear/seq/internal/adapter/binance"
	"github.com/BullionBear/seq/internal/adapter/bybit"
	"github.com/BullionBear/seq/internal/srv/catalog"
)

type DataClientRouter struct {
	catalog *catalog.Catalog

	binanceSpotDataClient *binance.BinanceSpotDataClient
	bybitSpotDataClient   *bybit.BybitDataClient
}

func (r *DataClientRouter) SubscribeDepth(symbolID int, limit int) (unsubscribe func(), err error) {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return nil, err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		return r.binanceSpotDataClient.SubscribeDepth(symbolID, limit)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		return r.bybitSpotDataClient.SubscribeDepth(symbolID, limit)
	default:
		return nil, fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
}

func (r *DataClientRouter) SubscribeTrade(symbolID int) (unsubscribe func(), err error) {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return nil, err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		return r.binanceSpotDataClient.SubscribeTrade(symbolID)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		return r.bybitSpotDataClient.SubscribeTrade(symbolID)
	default:
		return nil, fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
}
