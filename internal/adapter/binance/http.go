package binance

import (
	"strconv"
	"strings"
	"time"

	"github.com/BullionBear/seq/internal/evbus"
	"github.com/BullionBear/seq/internal/srv/catalog"
	"github.com/BullionBear/seq/pkg/model"
	"github.com/bytedance/sonic"
	"github.com/valyala/fasthttp"
)

type BinanceHTTPClient struct {
	catalog  *catalog.Catalog
	eventBus *evbus.EventBus
	client   fasthttp.Client
	builder  strings.Builder
}

func NewBinanceHTTPClient(catalog *catalog.Catalog, eventBus *evbus.EventBus) BinanceHTTPClient {
	return BinanceHTTPClient{
		catalog:  catalog,
		eventBus: eventBus,
		client: fasthttp.Client{
			MaxConnsPerHost: 100,
		},
	}
}

// GetDepth fetches order book depth from Binance API
// Uses zero-allocation approach with strings.Builder for URL construction
func (c *BinanceHTTPClient) ReqDepth(symbolId int, limit int) error {
	// Build URL using strings.Builder (zero-allocation string concatenation)
	symbol, err := c.catalog.GetSymbol(symbolId)
	if err != nil {
		return err
	}
	c.builder.Reset()
	c.builder.Grow(len(BaseURL) + len(EndpointDepth) + len(symbol.Name) + 32) // Pre-allocate capacity
	c.builder.WriteString(BaseURL)
	c.builder.WriteString(EndpointDepth)
	c.builder.WriteByte('?')
	c.builder.WriteString("symbol=")
	c.builder.WriteString(symbol.Name)
	if limit > 0 {
		c.builder.WriteString("&limit=")
		c.builder.WriteString(strconv.Itoa(limit))
	}

	// Acquire request and response from fasthttp's internal pools (zero-allocation)
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// Set request URI and method
	req.SetRequestURI(c.builder.String())
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("Accept", "application/json")

	// Execute the request
	err = c.client.Do(req, resp)
	if err != nil {
		return err
	}

	// Check response status
	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK {
		return &HTTPError{
			StatusCode: statusCode,
			Body:       string(resp.Body()),
		}
	}

	// Deserialize depth response
	var binanceDepth binanceDepthResponse
	err = sonic.Unmarshal(resp.Body(), &binanceDepth)
	if err != nil {
		return err
	}

	// Allocate price levels using event bus
	asksCount := len(binanceDepth.Asks)
	bidsCount := len(binanceDepth.Bids)
	totalLevels := asksCount + bidsCount
	priceLevels := c.eventBus.AllocPriceLevels(totalLevels)

	// Parse asks
	asks := priceLevels[:asksCount]
	for i, ask := range binanceDepth.Asks {
		if len(ask) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(ask[0], 64)
		if err != nil {
			return err
		}
		quantity, err := strconv.ParseFloat(ask[1], 64)
		if err != nil {
			return err
		}
		asks[i] = model.PriceLevel{
			Price:    price,
			Quantity: quantity,
		}
	}

	// Parse bids
	bids := priceLevels[asksCount : asksCount+bidsCount]
	for i, bid := range binanceDepth.Bids {
		if len(bid) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(bid[0], 64)
		if err != nil {
			return err
		}
		quantity, err := strconv.ParseFloat(bid[1], 64)
		if err != nil {
			return err
		}
		bids[i] = model.PriceLevel{
			Price:    price,
			Quantity: quantity,
		}
	}

	// Create depth snapshot
	depthSnapshot := model.DepthSnapshot{
		SymbolID:  symbolId,
		DepthID:   int(binanceDepth.LastUpdateID),
		Timestamp: uint64(time.Now().UnixNano()),
		Asks:      asks,
		Bids:      bids,
	}

	// Publish to event bus
	c.eventBus.PublishDepthSnapshot(depthSnapshot)
	return nil
}

func (c *BinanceHTTPClient) ReqCreateOrder(acctID int, symbolId int, orderType model.OrderType, side model.Side, timeInForce model.TimeInForce, quantity float64, price float64) error {
	symbol, err := c.catalog.GetSymbol(symbolId)
	if err != nil {
		return err
	}

	acct, err := c.catalog.GetAccount(acctID)
	if err != nil {
		return err
	}
}
