package bybit

const (
	BaseURL = "https://api.bybit.com"

	// Bybit V5 API unifies all products into single endpoints
	// Use "category" parameter to specify: spot, linear, inverse, option
	EndpointOrderbook = "/v5/market/orderbook"

	// WebSocket URLs - Bybit requires separate connections per channel type
	BaseWsURL       = "wss://stream.bybit.com/v5/public"
	WsURLSpot       = BaseWsURL + "/spot"
	WsURLLinear     = BaseWsURL + "/linear"
	WsURLInverse    = BaseWsURL + "/inverse"
	WsURLOption     = BaseWsURL + "/option"
)
