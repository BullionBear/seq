package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/BullionBear/seq/core/catalog/cpanel"
	"github.com/BullionBear/seq/core/logger"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

type Catalog struct {
	cpanelClient *cpanel.CpanelClient
	symbols      map[int]cpanel.Symbol   // symbolID -> symbol
	exchanges    map[int]cpanel.Exchange // exchangeID -> exchange
	products     map[int]cpanel.Product  // productID -> product
	tokens       map[int]cpanel.Token    // tokenID -> token

	accounts map[int]cpanel.Account // accountID -> account

	// exchangeID -> symbolName -> symbol (for O(1) lookup by exchange and name)
	symbolsByExchangeAndName map[int]map[string]cpanel.Symbol
	exchangeNameToID         map[string]int // lowercase exchange name -> exchangeID (for case-insensitive lookup)
	tokenNameToID            map[string]int // lowercase token name -> tokenID (for case-insensitive lookup)
}

func NewCatalog(baseURL string, apiToken string) *Catalog {
	cpanelClient := cpanel.NewCpanelClient(baseURL, apiToken)
	catalog := Catalog{
		cpanelClient:             cpanelClient,
		symbols:                  make(map[int]cpanel.Symbol, 1024),
		exchanges:                make(map[int]cpanel.Exchange, 64),
		products:                 make(map[int]cpanel.Product, 16),
		tokens:                   make(map[int]cpanel.Token, 256),
		accounts:                 make(map[int]cpanel.Account, 1024),
		symbolsByExchangeAndName: make(map[int]map[string]cpanel.Symbol),
		exchangeNameToID:         make(map[string]int, 64),
		tokenNameToID:            make(map[string]int, 256),
	}
	if err := catalog.LoadAllSymbols(); err != nil {
		log().Error().Err(err).Msg("Failed to load all symbols")
		return nil
	}
	if err := catalog.LoadAllAccounts(); err != nil {
		log().Error().Err(err).Msg("Failed to load all accounts")
		return nil
	}
	return &catalog
}

func (c *Catalog) LoadAllSymbols() error {
	symbols, err := c.cpanelClient.GetSymbol(context.Background(), cpanel.SymbolParams{})
	if err != nil {
		return err
	}
	for _, symbol := range symbols {
		c.symbols[symbol.ID] = symbol
		log().Info().Msgf("Loaded symbol: %s(%d)", symbol.UniversalTicker, symbol.ID)
		c.exchanges[symbol.Exchange.ID] = symbol.Exchange
		c.exchangeNameToID[strings.ToLower(symbol.Exchange.Name)] = symbol.Exchange.ID
		if c.symbolsByExchangeAndName[symbol.Exchange.ID] == nil {
			c.symbolsByExchangeAndName[symbol.Exchange.ID] = make(map[string]cpanel.Symbol)
		}
		c.symbolsByExchangeAndName[symbol.Exchange.ID][symbol.Name] = symbol
		c.products[symbol.Product.ID] = symbol.Product
		c.tokens[symbol.BaseToken.ID] = symbol.BaseToken
		c.tokens[symbol.QuoteToken.ID] = symbol.QuoteToken
		c.tokenNameToID[symbol.BaseToken.Name] = symbol.BaseToken.ID
		c.tokenNameToID[symbol.QuoteToken.Name] = symbol.QuoteToken.ID
	}
	log().Info().
		Int("symbols", len(c.symbols)).
		Int("exchanges", len(c.exchanges)).
		Int("products", len(c.products)).
		Int("tokens", len(c.tokens)).
		Msg("Loaded catalog data")
	return nil
}

func (c *Catalog) LoadAllAccounts() error {
	accounts, err := c.cpanelClient.GetAccounts(context.Background())
	if err != nil {
		return err
	}
	for _, account := range accounts {
		c.accounts[account.ID] = account
	}
	log().Info().
		Int("accounts", len(c.accounts)).
		Msg("Loaded accounts")
	return nil
}

func (c *Catalog) GetSymbol(symbolID int) (*cpanel.Symbol, error) {
	symbol, ok := c.symbols[symbolID]
	if !ok {
		return nil, fmt.Errorf("symbol not found for symbolID: %d", symbolID)
	}
	return &symbol, nil
}

func (c *Catalog) GetToken(tokenID int) (*cpanel.Token, error) {
	token, ok := c.tokens[tokenID]
	if !ok {
		return nil, fmt.Errorf("token not found for tokenID: %d", tokenID)
	}
	return &token, nil
}

func (c *Catalog) GetTokenIDByName(name string) (int, error) {
	tokenID, ok := c.tokenNameToID[name]
	if !ok {
		return 0, fmt.Errorf("token not found for name: %s", name)
	}
	return tokenID, nil
}

func (c *Catalog) GetSymbolByUniversalTicker(universalTicker string) (*cpanel.Symbol, error) {
	for _, symbol := range c.symbols {
		if symbol.UniversalTicker == universalTicker {
			return &symbol, nil
		}
	}
	return nil, fmt.Errorf("symbol not found for universalTicker: %s", universalTicker)
}

// GetSymbolByExchangeAndName returns the symbol for the given exchange name and symbol name (e.g. "Binance", "BTCUSDT").
// Exchange name comparison is case-insensitive. Uses exchangeID -> symbolName -> symbol index for O(1) lookup.
func (c *Catalog) GetSymbolByExchangeAndName(exchangeName, symbolName string) (*cpanel.Symbol, error) {
	exchangeID, ok := c.exchangeNameToID[strings.ToLower(exchangeName)]
	if !ok {
		return nil, fmt.Errorf("symbol not found for exchange %s and name %s", exchangeName, symbolName)
	}
	byName, ok := c.symbolsByExchangeAndName[exchangeID]
	if !ok {
		return nil, fmt.Errorf("symbol not found for exchange %s and name %s", exchangeName, symbolName)
	}
	symbol, ok := byName[symbolName]
	if !ok {
		return nil, fmt.Errorf("symbol not found for exchange %s and name %s", exchangeName, symbolName)
	}
	return &symbol, nil
}

func (c *Catalog) GetAccount(accountID int) (*cpanel.Account, error) {
	account, ok := c.accounts[accountID]
	if !ok {
		return nil, fmt.Errorf("account not found for accountID: %d", accountID)
	}
	return &account, nil
}

// GetAccountByName returns the account with the given name, or nil if not found.
func (c *Catalog) GetAccountByName(name string) *cpanel.Account {
	for _, account := range c.accounts {
		if account.Name == name {
			return &account
		}
	}
	return nil
}
