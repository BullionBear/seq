package adapter

import (
	"context"
	"fmt"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/internal/adapter/binance"
	"github.com/BullionBear/seq/internal/adapter/bybit"
	"github.com/BullionBear/seq/internal/evbus"
)

type DataRouter struct {
	catalog  *catalog.Catalog
	eventBus *evbus.EventBus

	binanceSpotDataClient *binance.BinanceSpotDataClient
	bybitDataClient       *bybit.BybitDataClient
}

func NewDataRouter(catalog *catalog.Catalog, eventBus *evbus.EventBus) *DataRouter {
	return &DataRouter{
		catalog:               catalog,
		eventBus:              eventBus,
		binanceSpotDataClient: binance.NewBinanceSpotDataClient(catalog, eventBus),
		bybitDataClient:       bybit.NewBybitDataClient(catalog, eventBus),
	}
}

func (r *DataRouter) SubscribeDepthUpdate(symbolID int) error {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		r.binanceSpotDataClient.SubscribeDepthUpdate(symbolID, nil)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		r.bybitDataClient.SubscribeDepthUpdate(symbolID, nil)
	default:
		return fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
	return nil
}

func (r *DataRouter) ReqDepthSnapshot(symbolID int) error {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		return r.binanceSpotDataClient.ReqDepthSnapshot(symbolID, 1000) // Request 1000 levels
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		return r.bybitDataClient.ReqDepthSnapshot(symbolID, 1000)
	default:
		return fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
}

func (r *DataRouter) SubscribeTrade(symbolID int) error {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		r.binanceSpotDataClient.SubscribeTrade(symbolID)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		r.bybitDataClient.SubscribeTrade(symbolID)
	default:
		return fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
	return nil
}

func (r *DataRouter) Connect(ctx context.Context) error {
	// Only connect clients that have subscriptions
	if r.binanceSpotDataClient.HasSub() {
		if err := r.binanceSpotDataClient.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect Binance spot data client: %w", err)
		}
	}

	if r.bybitDataClient.HasSub() {
		if err := r.bybitDataClient.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect Bybit data client: %w", err)
		}
	}

	return nil
}

func (r *DataRouter) Disconnect() {
	// Only disconnect clients that were connected
	if r.binanceSpotDataClient.HasSub() {
		r.binanceSpotDataClient.Disconnect()
	}
	if r.bybitDataClient.HasSub() {
		r.bybitDataClient.Disconnect()
	}
}
