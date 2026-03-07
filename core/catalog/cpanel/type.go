package cpanel

import (
	"fmt"

	"github.com/BullionBear/seq/core/model/common"
)

// Symbol represents a trading symbol from the LynxLinkage API
type Symbol struct {
	ID              int      `json:"id"`
	Name            string   `json:"name"`
	Exchange        Exchange `json:"exchange"`
	Product         Product  `json:"product"`
	BaseToken       Token    `json:"base_token"`
	QuoteToken      Token    `json:"quote_token"`
	PricePrecision  int      `json:"price_precision"`
	SizePrecision   int      `json:"size_precision"`
	UniversalTicker string   `json:"universal_ticker"`
}

// Exchange represents exchange information
type Exchange struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Product represents product type information (e.g., SPOT)
type Product struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Token represents token/asset information
type Token struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// SymbolParams represents query parameters for the symbol endpoint
type SymbolParams struct {
	ExchangeID   *int    `json:"exchange_id,omitempty"`
	ExchangeSlug *string `json:"exchange_slug,omitempty"`
	ProductID    *int    `json:"product_id,omitempty"`
	ProductSlug  *string `json:"product_slug,omitempty"`
	BaseTokenID  *int    `json:"base_token_id,omitempty"`
	QuoteTokenID *int    `json:"quote_token_id,omitempty"`
}

// APIType represents the type of API authentication
type APIType string

const (
	APITypeHMAC    APIType = "HMAC"
	APITypeRSA     APIType = "RSA"
	APITypeED25519 APIType = "ED25519"
)

// Account represents an exchange account from the v2 API
type Account struct {
	ID        int             `json:"id"`
	UID       int             `json:"uid"`
	ExchID    common.Exchange `json:"exch_id"`
	Exchange  string          `json:"exchange"`
	Name      string          `json:"name"`
	ParentUID int             `json:"parent_uid"`
	apiKeys   []APIKey
	wallets   []Wallet
}

// GetAPI returns the API key with the given name, or an error if not found
func (a *Account) GetAPI(name string) (*APIKey, error) {
	for i := range a.apiKeys {
		if a.apiKeys[i].Name == name {
			return &a.apiKeys[i], nil
		}
	}
	return nil, fmt.Errorf("API key not found: %s", name)
}

// GetWallet returns the wallet with the given name, or an error if not found
func (a *Account) GetWallet(name string) (*Wallet, error) {
	for i := range a.wallets {
		if a.wallets[i].Name == name {
			return &a.wallets[i], nil
		}
	}
	return nil, fmt.Errorf("wallet not found: %s", name)
}

// SetAPIKeys sets the API keys for this account (used by catalog)
func (a *Account) SetAPIKeys(keys []APIKey) {
	a.apiKeys = keys
}

// SetWallets sets the wallets for this account (used by catalog)
func (a *Account) SetWallets(wallets []Wallet) {
	a.wallets = wallets
}

// APIKey represents API credentials from the v2 API
type APIKey struct {
	ID         int             `json:"id"`
	UserID     int             `json:"user_id"`
	ExchID     common.Exchange `json:"-"`
	UID        int             `json:"uid"`
	Name       string          `json:"name"`
	APIType    APIType         `json:"api_type"`
	Key        string          `json:"api_key"`
	Secret     string          `json:"api_secret"`
	Passphrase string          `json:"passphrase"`
	ParentUID  int             `json:"parent_uid"`
	CreatedAt  string          `json:"created_at"`
}

// Wallet represents a wallet from the v2 API
type Wallet struct {
	ID         int               `json:"id"`
	Name       string            `json:"name"`
	WalletType common.WalletType `json:"category_id"`
	AcctID     int               `json:"acct_id"`
	UserID     int               `json:"user_id"`
	IsActive   bool              `json:"is_active"`
}
