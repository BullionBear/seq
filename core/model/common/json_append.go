package common

import "github.com/BullionBear/seq/core/model/codec"

// AppendJSON appends a PriceLevel as a JSON object matching encoding/json field names.
func (p PriceLevel) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"Price":`...)
	dst = codec.AppendJSONFloat(dst, p.Price)
	dst = append(dst, `,"Quantity":`...)
	dst = codec.AppendJSONFloat(dst, p.Quantity)
	dst = append(dst, `,"PriceTick":`...)
	dst = codec.AppendJSONInt(dst, int64(p.PriceTick))
	dst = append(dst, `,"QuantityTick":`...)
	dst = codec.AppendJSONInt(dst, int64(p.QuantityTick))
	return append(dst, '}')
}

// AppendJSON appends a Balance as a JSON object matching encoding/json field names.
func (b Balance) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"TokenID":`...)
	dst = codec.AppendJSONInt(dst, int64(b.TokenID))
	dst = append(dst, `,"Available":`...)
	dst = codec.AppendJSONFloat(dst, b.Available)
	dst = append(dst, `,"Locked":`...)
	dst = codec.AppendJSONFloat(dst, b.Locked)
	dst = append(dst, `,"Total":`...)
	dst = codec.AppendJSONFloat(dst, b.Total)
	return append(dst, '}')
}

// AppendPriceLevelsJSON appends a []PriceLevel (nil → null).
func AppendPriceLevelsJSON(dst []byte, levels []PriceLevel) []byte {
	if levels == nil {
		return append(dst, "null"...)
	}
	dst = append(dst, '[')
	for i, l := range levels {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = l.AppendJSON(dst)
	}
	return append(dst, ']')
}

// AppendBalancesJSON appends a []Balance (nil → null).
func AppendBalancesJSON(dst []byte, balances []Balance) []byte {
	if balances == nil {
		return append(dst, "null"...)
	}
	dst = append(dst, '[')
	for i, b := range balances {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = b.AppendJSON(dst)
	}
	return append(dst, ']')
}
