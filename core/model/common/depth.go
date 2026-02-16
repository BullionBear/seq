package common

import "math"

// PriceLevel represents a single price level in an order book.
// PriceTick and QuantityTick are integer representations using symbol precision:
// PriceTick = round(Price * 10^PricePrecision), e.g. 71532.32 with precision 2 -> 7153232
// QuantityTick = round(Quantity * 10^SizePrecision), e.g. 0.025 with precision 5 -> 2500
type PriceLevel struct {
	Price        float64
	Quantity     float64
	PriceTick    int
	QuantityTick int
}

// PriceToTick converts price to integer tick given precision.
// E.g. PriceToTick(71532.32, 2) = 7153232
func PriceToTick(price float64, pricePrecision int) int {
	multiplier := math.Pow(10, float64(pricePrecision))
	return int(math.Round(price * multiplier))
}

// QuantityToTick converts quantity to integer tick given precision.
// E.g. QuantityToTick(0.025, 5) = 2500
func QuantityToTick(quantity float64, sizePrecision int) int {
	multiplier := math.Pow(10, float64(sizePrecision))
	return int(math.Round(quantity * multiplier))
}
