package bybit

const (
	BaseURL = "https://api.bybit.com"

	// Bybit V5 API unifies all products into single endpoints
	// Use "category" parameter to specify: spot, linear, inverse, option
	EndpointOrderbook     = "/v5/market/orderbook"
	EndpointWalletBalance = "/v5/account/wallet-balance"

	// HTTP API settings
	HTTPRecvWindow = "5000" // 5 seconds

	// WebSocket URLs - Bybit requires separate connections per channel type
	// Public market data streams (per category)
	BaseWsURL   = "wss://stream.bybit.com/v5/public"
	WsURLSpot   = BaseWsURL + "/spot"
	WsURLLinear = BaseWsURL + "/linear"

	WsURLInverse = BaseWsURL + "/inverse"
	WsURLOption  = BaseWsURL + "/option"

	// Private WebSocket streams (authentication required)
	// Private stream: for listening to order updates, executions, wallet updates
	WsPrivateURL = "wss://stream.bybit.com/v5/private"
	// Trade stream: for sending orders via WebSocket
	WsTradeURL = "wss://stream.bybit.com/v5/trade"

	// Testnet WebSocket URLs
	WsPrivateURLTestnet = "wss://stream-testnet.bybit.com/v5/private"
	WsTradeURLTestnet   = "wss://stream-testnet.bybit.com/v5/trade"
)
