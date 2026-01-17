package catalog

import (
	"context"
	"fmt"

	"github.com/BullionBear/seq/internal/srv/catalog/cpanel"
	"github.com/BullionBear/seq/pkg/logger"
)

var log = logger.Get()

type Catalog struct {
	cpanelClient *cpanel.CpanelClient
	symbols      map[int]cpanel.Symbol
	exchanges    map[int]cpanel.Exchange
	products     map[int]cpanel.Product
	tokens       map[int]cpanel.Token

	accounts map[int]cpanel.Account
}

func NewCatalog(baseURL string, apiToken string) *Catalog {
	cpanelClient := cpanel.NewCpanelClient(baseURL, apiToken)
	catalog := Catalog{
		cpanelClient: cpanelClient,
		symbols:      make(map[int]cpanel.Symbol, 1024),
		exchanges:    make(map[int]cpanel.Exchange, 64),
		products:     make(map[int]cpanel.Product, 16),
		tokens:       make(map[int]cpanel.Token, 256),
		accounts:     make(map[int]cpanel.Account, 1024),
	}
	if err := catalog.LoadAllSymbols(); err != nil {
		log.Error().Err(err).Msg("Failed to load all symbols")
		return nil
	}
	if err := catalog.LoadAllAccounts(); err != nil {
		log.Error().Err(err).Msg("Failed to load all accounts")
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

func (c *Catalog) LoadAllAccounts() error {
	accounts, err := c.cpanelClient.GetAccounts(context.Background())
	if err != nil {
		return err
	}
	for _, account := range accounts {
		c.accounts[account.ID] = account
	}
	log.Info().
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

func (c *Catalog) GetAccount(accountID int) (*cpanel.Account, error) {
	account, ok := c.accounts[accountID]
	if !ok {
		return nil, fmt.Errorf("account not found for accountID: %d", accountID)
	}
	return &account, nil
}
