package catalog

import (
	"context"
	"fmt"

	"github.com/BullionBear/seq/pkg/integration/lynxlinkage"
	"github.com/BullionBear/seq/pkg/logger"
)

var log = logger.Get()

type Catalog struct {
	lynxlinkageClient *lynxlinkage.LynxLinkageClient
	symbols           map[int]lynxlinkage.Symbol
	exchanges         map[int]lynxlinkage.Exchange
	products          map[int]lynxlinkage.Product
	tokens            map[int]lynxlinkage.Token
}

func NewCatalog(baseURL string) *Catalog {
	lynxlinkageClient := lynxlinkage.NewLynxLinkageClient(baseURL)
	catalog := Catalog{
		lynxlinkageClient: lynxlinkageClient,
		symbols:           make(map[int]lynxlinkage.Symbol, 1024),
		exchanges:         make(map[int]lynxlinkage.Exchange, 64),
		products:          make(map[int]lynxlinkage.Product, 16),
		tokens:            make(map[int]lynxlinkage.Token, 256),
	}
	if err := catalog.LoadAllSymbols(); err != nil {
		log.Error().Err(err).Msg("Failed to load all symbols")
		return nil
	}
	return &catalog
}

func (c *Catalog) LoadAllSymbols() error {
	symbols, err := c.lynxlinkageClient.GetSymbol(context.Background(), lynxlinkage.SymbolParams{})
	if err != nil {
		return err
	}
	for _, symbol := range symbols {
		c.symbols[symbol.ID] = symbol
		c.exchanges[symbol.Exchange.ID] = symbol.Exchange
		c.products[symbol.Product.ID] = symbol.Product
		c.tokens[symbol.BaseToken.ID] = symbol.BaseToken
		c.tokens[symbol.QuoteToken.ID] = symbol.QuoteToken
	}
	log.Info().
		Int("symbols", len(c.symbols)).
		Int("exchanges", len(c.exchanges)).
		Int("products", len(c.products)).
		Int("tokens", len(c.tokens)).
		Msg("Loaded catalog data")
	return nil
}

func (c *Catalog) GetSymbol(symbolID int) (*lynxlinkage.Symbol, error) {
	symbol, ok := c.symbols[symbolID]
	if !ok {
		return nil, fmt.Errorf("symbol not found for symbolID: %d", symbolID)
	}
	return &symbol, nil
}
