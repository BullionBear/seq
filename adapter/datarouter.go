package adapter

import (
	"context"
	"fmt"

	"github.com/BullionBear/seq/adapter/binance"
	"github.com/BullionBear/seq/adapter/binancefutures"
	"github.com/BullionBear/seq/adapter/bybit"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/msgbus"
)

// DataRouterEntry is per-symbol data subscription config from YAML.
type DataRouterEntry struct {
	Symbol   string          `yaml:"symbol"`             // Universal ticker (required)
	Endpoint string          `yaml:"endpoint,omitempty"` // Regional endpoint: bybit, bybit_tr, bybit_eu
	Depth    *DepthConfig    `yaml:"depth,omitempty"`    // Depth subscription options
	Trade    *TradeConfig    `yaml:"trade,omitempty"`    // Trade tick subscription
	AggTrade *AggTradeConfig `yaml:"aggTrade,omitempty"` // Aggregate trade subscription
	Kline    *KlineConfig    `yaml:"kline,omitempty"`    // Kline subscription options
}

// DepthConfig configures depth stream subscription.
type DepthConfig struct {
	// Type selects the depth stream kind for Binance:
	//   delta (default) → diff depth (@depth@100ms)
	//   depth5|depth10|depth20 → partial book WS (@depthN@100ms → DepthSnapshot)
	Type     string `yaml:"type,omitempty"`
	PushRate string `yaml:"push_rate,omitempty"` // 100ms, 1000ms (binance)
	Levels   int    `yaml:"levels,omitempty"`    // 1, 50, 200, 1000 (bybit)
}

// TradeConfig enables trade tick stream subscription.
// Presence of the key is enough; no options today.
type TradeConfig struct{}

// AggTradeConfig enables aggregate trade stream subscription (Binance @aggTrade).
// Presence of the key is enough; no options today. Ignored on venues without aggTrade.
type AggTradeConfig struct{}

// KlineConfig configures kline / candlestick stream subscription.
type KlineConfig struct {
	Interval string `yaml:"interval,omitempty"` // 1m, 5m, 1h, 1d, ... (Binance form; Bybit mapped)
}

// DepthOptions contains generic depth subscription options.
// These are translated to primitive parameters by the router.
type DepthOptions struct {
	Type     string // delta, depth5, depth10, depth20 (binance)
	PushRate string // 100ms, 1000ms (binance)
	Levels   int    // 1, 50, 200, 1000 (bybit)
}

// KlineOptions contains generic kline subscription options.
type KlineOptions struct {
	Interval string // 1m, 5m, 1h, 1d, ...
}

// DataClient is the interface for exchange-specific market data stream clients.
// Parameters use primitives so implementations need not import this package.
type DataClient interface {
	HasSub() bool
	Connect(ctx context.Context) error
	Disconnect()
	SubscribeDepthUpdate(symbolID int, depthLevel int, pushRateMs int)
	SubscribeTrade(symbolID int)
	SubscribeAggTrade(symbolID int)
	SubscribeKline(symbolID int, interval string)
	ReqDepthSnapshot(symbolID int, limit int) error
	// ReqHistoricalKline fetches historical candles. Times are nanoseconds; 0 omits the bound.
	ReqHistoricalKline(symbolID int, interval string, startTimeNs, endTimeNs uint64, limit int) error
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
	r.RegisterFactory(int(common.ExchangeBinance), int(common.ProductTypePerpetual), func(c *catalog.Catalog, m *msgbus.MsgBus) DataClient {
		return binancefutures.NewBinanceFuturesDataClient(c, m)
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
		// Binance partial-book types override depthLevel; Bybit ignores these
		// values (only 1/50/200/1000 are valid Bybit depths).
		switch opts.Type {
		case "depth5":
			depthLevel = 5
		case "depth10":
			depthLevel = 10
		case "depth20":
			depthLevel = 20
		}
		switch opts.PushRate {
		case "1000ms", "1s":
			pushRateMs = 1000
		}
	}

	client.SubscribeDepthUpdate(symbolID, depthLevel, pushRateMs)
	return nil
}

// SubscribeTrade subscribes to trade tick updates for a symbol.
func (r *DataRouter) SubscribeTrade(symbolID int) error {
	symbol, err := r.cat.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	client, err := r.getOrCreateClient(symbol.Exchange.ID, symbol.Product.ID)
	if err != nil {
		return err
	}
	client.SubscribeTrade(symbolID)
	return nil
}

// SubscribeAggTrade subscribes to aggregate trade updates for a symbol.
func (r *DataRouter) SubscribeAggTrade(symbolID int) error {
	symbol, err := r.cat.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	client, err := r.getOrCreateClient(symbol.Exchange.ID, symbol.Product.ID)
	if err != nil {
		return err
	}
	client.SubscribeAggTrade(symbolID)
	return nil
}

// SubscribeKline subscribes to kline updates for a symbol with options.
func (r *DataRouter) SubscribeKline(symbolID int, opts *KlineOptions) error {
	symbol, err := r.cat.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	client, err := r.getOrCreateClient(symbol.Exchange.ID, symbol.Product.ID)
	if err != nil {
		return err
	}

	interval := "1m"
	if opts != nil && opts.Interval != "" {
		interval = opts.Interval
	}
	client.SubscribeKline(symbolID, interval)
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

// ReqHistoricalKline requests historical klines for a symbol.
// interval is a Binance-style string (e.g. "1m"); start/end are nanoseconds (0 = omit).
func (r *DataRouter) ReqHistoricalKline(symbolID int, interval string, startTimeNs, endTimeNs uint64, limit int) error {
	symbol, err := r.cat.GetSymbol(symbolID)
	if err != nil {
		return err
	}
	client, err := r.getOrCreateClient(symbol.Exchange.ID, symbol.Product.ID)
	if err != nil {
		return err
	}
	return client.ReqHistoricalKline(symbolID, interval, startTimeNs, endTimeNs, limit)
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
