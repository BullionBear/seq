package cpanel

import "time"

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

// Account represents an API key response from the LynxLinkage API
type Account struct {
	ID         int       `json:"id"`
	UserID     int       `json:"user_id"`
	Exchange   string    `json:"exchange"`
	UID        string    `json:"uid"`
	Name       string    `json:"name"`
	APIType    APIType   `json:"api_type"`
	APIKey     string    `json:"api_key"`
	APISecret  string    `json:"api_secret"`
	Passphrase string    `json:"passphrase"`
	MasterUID  string    `json:"master_uid"`
	CreatedAt  time.Time `json:"created_at"`
}
