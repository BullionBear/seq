package adapter

import (
	"context"
	"fmt"

	"github.com/BullionBear/seq/adapter/binance"
	"github.com/BullionBear/seq/adapter/bybit"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/msgbus"
)

// DataRouterEntry is per-symbol data subscription config from YAML.
type DataRouterEntry struct {
	Symbol   string       `yaml:"symbol"`             // Universal ticker (required)
	Endpoint string       `yaml:"endpoint,omitempty"` // Regional endpoint: bybit, bybit_tr, bybit_eu
	Depth    *DepthConfig `yaml:"depth,omitempty"`    // Depth subscription options
	Trade    *TradeConfig `yaml:"trade,omitempty"`    // Trade subscription options
}

// DepthConfig configures depth stream subscription.
type DepthConfig struct {
	Type     string `yaml:"type,omitempty"`      // delta, snapshot, depth5, depth10, depth20 (binance)
	PushRate string `yaml:"push_rate,omitempty"` // 100ms, 1000ms (binance)
	Levels   int    `yaml:"levels,omitempty"`    // 1, 50, 200, 500 (bybit)
}

// TradeConfig configures trade stream subscription.
type TradeConfig struct {
	Type string `yaml:"type,omitempty"` // trade, aggTrade
}

// DepthOptions contains generic depth subscription options.
// These are translated to primitive parameters by the router.
type DepthOptions struct {
	Type     string // delta, snapshot, depth5, depth10, depth20 (binance)
	PushRate string // 100ms, 1000ms (binance)
	Levels   int    // 1, 50, 200, 500 (bybit)
}

// TradeOptions contains generic trade subscription options.
type TradeOptions struct {
	Type string // trade, aggTrade
}

// DataClient is the interface for exchange-specific market data stream clients.
// Parameters use primitives so implementations need not import this package.
type DataClient interface {
	HasSub() bool
	Connect(ctx context.Context) error
	Disconnect()
	SubscribeDepthUpdate(symbolID int, depthLevel int, pushRateMs int)
	SubscribeTrade(symbolID int, useAggTrade bool)
	ReqDepthSnapshot(symbolID int, limit int) error
}

// DataClientFactory creates a DataClient for a specific exchange+product.
type DataClientFactory func(cat *catalog.Catalog, bus *msgbus.MsgBus) DataClient

type clientKey struct {
	exchangeID int
	productID  int
}

type DataRouter struct {
	cat       *catalog.Catalog
	bus       *msgbus.MsgBus
	factories map[clientKey]DataClientFactory
	clients   map[clientKey]DataClient
}

func NewDataRouter(cat *catalog.Catalog, bus *msgbus.MsgBus) *DataRouter {
	r := &DataRouter{
		cat:       cat,
		bus:       bus,
		factories: make(map[clientKey]DataClientFactory),
		clients:   make(map[clientKey]DataClient),
	}

	r.RegisterFactory(int(common.ExchangeBinance), int(common.ProductTypeSpot), func(c *catalog.Catalog, m *msgbus.MsgBus) DataClient {
		return binance.NewBinanceSpotDataClient(c, m)
	})
	r.RegisterFactory(int(common.ExchangeBybit), int(common.ProductTypeSpot), func(c *catalog.Catalog, m *msgbus.MsgBus) DataClient {
		return bybit.NewBybitDataClient(c, m)
	})

	return r
}

// RegisterFactory registers a factory for a given exchange+product pair.
func (r *DataRouter) RegisterFactory(exchangeID, productID int, factory DataClientFactory) {
	r.factories[clientKey{exchangeID, productID}] = factory
}

// getOrCreateClient lazily creates a data client for the exchange+product.
func (r *DataRouter) getOrCreateClient(exchangeID, productID int) (DataClient, error) {
	key := clientKey{exchangeID, productID}
	if client, ok := r.clients[key]; ok {
		return client, nil
	}
	factory, ok := r.factories[key]
	if !ok {
		return nil, fmt.Errorf("no data client factory for exchange=%d product=%d", exchangeID, productID)
	}
	client := factory(r.cat, r.bus)
	r.clients[key] = client
	return client, nil
}

// SubscribeDepthUpdate subscribes to depth updates for a symbol with options.
func (r *DataRouter) SubscribeDepthUpdate(symbolID int, opts *DepthOptions) error {
	symbol, err := r.cat.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	client, err := r.getOrCreateClient(symbol.Exchange.ID, symbol.Product.ID)
	if err != nil {
		return err
	}

	depthLevel := 50
	pushRateMs := 100
	if opts != nil {
		if opts.Levels > 0 {
			depthLevel = opts.Levels
		}
		switch opts.PushRate {
		case "1000ms", "1s":
			pushRateMs = 1000
		}
	}

	client.SubscribeDepthUpdate(symbolID, depthLevel, pushRateMs)
	return nil
}

// SubscribeTrade subscribes to trade updates for a symbol with options.
func (r *DataRouter) SubscribeTrade(symbolID int, opts *TradeOptions) error {
	symbol, err := r.cat.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	client, err := r.getOrCreateClient(symbol.Exchange.ID, symbol.Product.ID)
	if err != nil {
		return err
	}

	useAggTrade := false
	if opts != nil && opts.Type == "aggTrade" {
		useAggTrade = true
	}

	client.SubscribeTrade(symbolID, useAggTrade)
	return nil
}

func (r *DataRouter) ReqDepthSnapshot(symbolID int) error {
	symbol, err := r.cat.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	client, err := r.getOrCreateClient(symbol.Exchange.ID, symbol.Product.ID)
	if err != nil {
		return err
	}
	return client.ReqDepthSnapshot(symbolID, 1000)
}

func (r *DataRouter) Connect(ctx context.Context) error {
	for _, client := range r.clients {
		if client.HasSub() {
			if err := client.Connect(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *DataRouter) Disconnect() {
	for _, client := range r.clients {
		if client.HasSub() {
			client.Disconnect()
		}
	}
}
