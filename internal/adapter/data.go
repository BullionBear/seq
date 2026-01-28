package adapter

import (
	"context"
	"fmt"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/internal/adapter/binance"
	"github.com/BullionBear/seq/internal/adapter/bybit"
	"github.com/BullionBear/seq/internal/evbus"
)

type DataClientRouter struct {
	catalog  *catalog.Catalog
	eventBus *evbus.EventBus

	binanceSpotDataClient binance.BinanceSpotDataClient
	bybitSpotDataClient   bybit.BybitDataClient
}

func NewDataClientRouter(catalog *catalog.Catalog, eventBus *evbus.EventBus) *DataClientRouter {
	return &DataClientRouter{
		catalog:               catalog,
		eventBus:              eventBus,
		binanceSpotDataClient: binance.NewBinanceSpotDataClient(),
		bybitSpotDataClient:   bybit.NewBybitDataClient(),
	}
}

func (r *DataClientRouter) SubscribeDepthDelta(symbolID int) error {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		r.binanceSpotDataClient.SubscribeDepthUpdate(symbolID)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		r.bybitSpotDataClient.SubscribeDepthUpdate(symbolID)
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
