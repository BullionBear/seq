package catalog

import (
	"fmt"
	"os"
	"strings"

	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/bytedance/sonic"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

type Catalog struct {
	symbols   map[int]Symbol   // symbolID -> symbol
	exchanges map[int]Exchange // exchangeID -> exchange
	products  map[int]Product  // productID -> product
	tokens    map[int]Token    // tokenID -> token

	accounts map[int]Account // accountID -> account

	// exchangeID -> symbolName -> symbol (for O(1) lookup by exchange and name)
	symbolsByExchangeAndName map[int]map[string]Symbol
	exchangeNameToID         map[string]int // lowercase exchange name -> exchangeID (for case-insensitive lookup)
	tokenNameToID            map[string]int // lowercase token name -> tokenID (for case-insensitive lookup)
}

// NewCatalog builds a catalog from a local instruments JSON file and the
// accounts defined in the configuration.
func NewCatalog(cfg Config) (*Catalog, error) {
	catalog := Catalog{
		symbols:                  make(map[int]Symbol, 1024),
		exchanges:                make(map[int]Exchange, 64),
		products:                 make(map[int]Product, 16),
		tokens:                   make(map[int]Token, 256),
		accounts:                 make(map[int]Account, 1024),
		symbolsByExchangeAndName: make(map[int]map[string]Symbol),
		exchangeNameToID:         make(map[string]int, 64),
		tokenNameToID:            make(map[string]int, 256),
	}
	if err := catalog.loadSymbolsFromFile(cfg.Instruments); err != nil {
		return nil, fmt.Errorf("failed to load instruments from %s: %w", cfg.Instruments, err)
	}
	if err := catalog.loadAccountsFromConfig(cfg.Accounts); err != nil {
		return nil, fmt.Errorf("failed to load accounts from config: %w", err)
	}
	return &catalog, nil
}

func (c *Catalog) loadSymbolsFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var symbols []Symbol
	if err := sonic.Unmarshal(data, &symbols); err != nil {
		return fmt.Errorf("failed to unmarshal instruments: %w", err)
	}
	for _, symbol := range symbols {
		c.symbols[symbol.ID] = symbol
		log().Info().Msgf("Loaded symbol: %s(%d)", symbol.UniversalTicker, symbol.ID)
		c.exchanges[symbol.Exchange.ID] = symbol.Exchange
		c.exchangeNameToID[strings.ToLower(symbol.Exchange.Name)] = symbol.Exchange.ID
		if c.symbolsByExchangeAndName[symbol.Exchange.ID] == nil {
			c.symbolsByExchangeAndName[symbol.Exchange.ID] = make(map[string]Symbol)
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

func (c *Catalog) loadAccountsFromConfig(accountConfigs []AccountConfig) error {
	walletID := 0
	apiKeyID := 0
	for i, ac := range accountConfigs {
		exchID, err := parseExchange(ac.Exchange)
		if err != nil {
			return fmt.Errorf("account %q: %w", ac.Name, err)
		}

		accountID := i + 1
		account := Account{
			ID:       accountID,
			UID:      accountID,
			ExchID:   exchID,
			Exchange: ac.Exchange,
			Name:     ac.Name,
		}

		apiKeys := make([]APIKey, 0, len(ac.APIKeys))
		for _, kc := range ac.APIKeys {
			apiType, err := parseAPIType(kc.Type)
			if err != nil {
				return fmt.Errorf("account %q, api key %q: %w", ac.Name, kc.Name, err)
			}
			apiKeyID++
			apiKeys = append(apiKeys, APIKey{
				ID:         apiKeyID,
				UID:        accountID,
				ExchID:     exchID,
				Name:       kc.Name,
				APIType:    apiType,
				Key:        kc.Key,
				Secret:     kc.Secret,
				Passphrase: kc.Passphrase,
			})
		}
		account.SetAPIKeys(apiKeys)

		wallets := make([]Wallet, 0, len(ac.Wallets))
		for _, wc := range ac.Wallets {
			walletType, err := parseWalletType(wc.Type)
			if err != nil {
				return fmt.Errorf("account %q, wallet %q: %w", ac.Name, wc.Name, err)
			}
			walletID++
			wallets = append(wallets, Wallet{
				ID:         walletID,
				Name:       wc.Name,
				WalletType: walletType,
				AcctID:     accountID,
				IsActive:   true,
			})
		}
		account.SetWallets(wallets)

		c.accounts[accountID] = account
		log().Info().
			Str("account", account.Name).
			Int("id", account.ID).
			Str("exchange", account.Exchange).
			Int("apiKeys", len(apiKeys)).
			Int("wallets", len(wallets)).
			Msg("Loaded account from config")
	}
	log().Info().Int("accounts", len(c.accounts)).Msg("Loaded accounts")
	return nil
}

func parseExchange(name string) (common.Exchange, error) {
	switch strings.ToLower(name) {
	case "binance":
		return common.ExchangeBinance, nil
	case "bybit":
		return common.ExchangeBybit, nil
	default:
		return common.ExchangeUnknown, fmt.Errorf("unknown exchange: %s", name)
	}
}

func parseAPIType(name string) (APIType, error) {
	switch strings.ToUpper(name) {
	case string(APITypeHMAC):
		return APITypeHMAC, nil
	case string(APITypeRSA):
		return APITypeRSA, nil
	case string(APITypeED25519):
		return APITypeED25519, nil
	default:
		return "", fmt.Errorf("unknown API type: %s", name)
	}
}

func parseWalletType(name string) (common.WalletType, error) {
	switch strings.ToLower(name) {
	case "spot":
		return common.WalletTypeSpot, nil
	case "umargin":
		return common.WalletTypeUMargin, nil
	case "cmargin":
		return common.WalletTypeCMargin, nil
	case "leverage":
		return common.WalletTypeLeverage, nil
	case "unified":
		return common.WalletTypeUnified, nil
	default:
		return common.WalletTypeUnknown, fmt.Errorf("unknown wallet type: %s", name)
	}
}

func (c *Catalog) GetSymbol(symbolID int) (*Symbol, error) {
	symbol, ok := c.symbols[symbolID]
	if !ok {
		return nil, fmt.Errorf("symbol not found for symbolID: %d", symbolID)
	}
	return &symbol, nil
}

func (c *Catalog) GetToken(tokenID int) (*Token, error) {
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

func (c *Catalog) GetSymbolByUniversalTicker(universalTicker string) (*Symbol, error) {
	for _, symbol := range c.symbols {
		if symbol.UniversalTicker == universalTicker {
			return &symbol, nil
		}
	}
	return nil, fmt.Errorf("symbol not found for universalTicker: %s", universalTicker)
}

// GetSymbolByExchangeAndName returns the symbol for the given exchange name and symbol name (e.g. "Binance", "BTCUSDT").
// Exchange name comparison is case-insensitive. Uses exchangeID -> symbolName -> symbol index for O(1) lookup.
func (c *Catalog) GetSymbolByExchangeAndName(exchangeName, symbolName string) (*Symbol, error) {
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

func (c *Catalog) GetAccount(accountID int) (*Account, error) {
	account, ok := c.accounts[accountID]
	if !ok {
		return nil, fmt.Errorf("account not found for accountID: %d", accountID)
	}
	return &account, nil
}

// GetWalletByName searches all accounts and returns the wallet with the given name.
func (c *Catalog) GetWalletByName(name string) (*Wallet, error) {
	for _, account := range c.accounts {
		if wallet, err := account.GetWallet(name); err == nil {
			return wallet, nil
		}
	}
	return nil, fmt.Errorf("wallet not found: %s", name)
}

// GetAccountByName returns the account with the given name, or nil if not found.
func (c *Catalog) GetAccountByName(name string) *Account {
	for _, account := range c.accounts {
		if account.Name == name {
			return &account
		}
	}
	return nil
}
