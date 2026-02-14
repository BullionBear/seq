package adapter

import (
	"context"
	"fmt"

	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/internal/adapter/binance"
	"github.com/BullionBear/seq/internal/adapter/bybit"
	"github.com/BullionBear/seq/internal/msgbus"
)

// DepthOptions contains generic depth subscription options
// These are translated to exchange-specific options by the router
type DepthOptions struct {
	Type     string // delta, snapshot, depth5, depth10, depth20 (binance)
	PushRate string // 100ms, 1000ms (binance)
	Levels   int    // 1, 50, 200, 500 (bybit)
}

// TradeOptions contains generic trade subscription options
type TradeOptions struct {
	Type string // trade, aggTrade
}

type DataRouter struct {
	catalog *catalog.Catalog
	msgBus  *msgbus.MsgBus

	binanceSpotDataClient *binance.BinanceSpotDataClient
	bybitDataClient       *bybit.BybitDataClient
}

func NewDataRouter(catalog *catalog.Catalog, msgBus *msgbus.MsgBus) *DataRouter {
	return &DataRouter{
		catalog:               catalog,
		msgBus:                msgBus,
		binanceSpotDataClient: binance.NewBinanceSpotDataClient(catalog, msgBus),
		bybitDataClient:       bybit.NewBybitDataClient(catalog, msgBus),
	}
}

// SubscribeDepthUpdate subscribes to depth updates for a symbol with options
func (r *DataRouter) SubscribeDepthUpdate(symbolID int, opts *DepthOptions) error {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		binanceOpts := r.toBinanceDepthOptions(opts)
		r.binanceSpotDataClient.SubscribeDepthUpdate(symbolID, binanceOpts)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		bybitOpts := r.toBybitDepthOptions(opts)
		r.bybitDataClient.SubscribeDepthUpdate(symbolID, bybitOpts)
	default:
		return fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
	return nil
}

// toBinanceDepthOptions converts generic DepthOptions to Binance-specific options
func (r *DataRouter) toBinanceDepthOptions(opts *DepthOptions) *binance.DepthSubscriptionOptions {
	if opts == nil {
		return nil
	}
	binanceOpts := &binance.DepthSubscriptionOptions{}
	switch opts.PushRate {
	case "100ms":
		binanceOpts.PushRate = binance.PushRate100ms
	case "1000ms", "1s":
		binanceOpts.PushRate = binance.PushRate1s
	default:
		binanceOpts.PushRate = binance.PushRate100ms // default
	}
	return binanceOpts
}

// toBybitDepthOptions converts generic DepthOptions to Bybit-specific options
func (r *DataRouter) toBybitDepthOptions(opts *DepthOptions) *bybit.DepthSubscriptionOptions {
	if opts == nil {
		return nil
	}
	bybitOpts := &bybit.DepthSubscriptionOptions{}
	switch opts.Levels {
	case 1:
		bybitOpts.Depth = bybit.DepthLevel1
	case 50:
		bybitOpts.Depth = bybit.DepthLevel50
	case 200:
		bybitOpts.Depth = bybit.DepthLevel200
	case 500, 1000:
		bybitOpts.Depth = bybit.DepthLevel1000
	default:
		bybitOpts.Depth = bybit.DepthLevel50 // default
	}
	return bybitOpts
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

// SubscribeTrade subscribes to trade updates for a symbol with options
func (r *DataRouter) SubscribeTrade(symbolID int, opts *TradeOptions) error {
	symbol, err := r.catalog.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	switch {
	case symbol.Exchange.ID == int(ExchangeBinance) && symbol.Product.ID == int(ProductTypeSpot):
		binanceOpts := r.toBinanceTradeOptions(opts)
		r.binanceSpotDataClient.SubscribeTrade(symbolID, binanceOpts)
	case symbol.Exchange.ID == int(ExchangeBybit) && symbol.Product.ID == int(ProductTypeSpot):
		r.bybitDataClient.SubscribeTrade(symbolID)
	default:
		return fmt.Errorf("unsupported exchange: %d", symbol.Exchange.ID)
	}
	return nil
}

// toBinanceTradeOptions converts generic TradeOptions to Binance-specific options
func (r *DataRouter) toBinanceTradeOptions(opts *TradeOptions) *binance.TradeSubscriptionOptions {
	if opts == nil {
		return nil
	}
	binanceOpts := &binance.TradeSubscriptionOptions{}
	switch opts.Type {
	case "aggTrade":
		binanceOpts.UseAggTrade = true
	default:
		binanceOpts.UseAggTrade = false
	}
	return binanceOpts
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
