package adapter

import (
	"context"
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

func (r *DataClientRouter) SubscribeDepth(symbolID int, limit int) error {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		r.binanceSpotDataClient.SubscribeDepth(symbolID, limit)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		r.bybitSpotDataClient.SubscribeDepth(symbolID, limit)
	default:
		return fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
	return nil
}

func (r *DataClientRouter) SubscribeTrade(symbolID int) error {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		r.binanceSpotDataClient.SubscribeTrade(symbolID)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		r.bybitSpotDataClient.SubscribeTrade(symbolID)
	default:
		return fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
	return nil
}

func (r *DataClientRouter) Connect(ctx context.Context) error {
	return nil
}

func (r *DataClientRouter) Disconnect() {
}
