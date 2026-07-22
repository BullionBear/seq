package codec

import "strconv"

// AppendJSONString appends a JSON-encoded string (including quotes) to dst.
// Alloc-free when dst has enough capacity.
func AppendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\\':
			dst = append(dst, '\\', c)
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
			} else {
				dst = append(dst, c)
			}
		}
	}
	return append(dst, '"')
}

// AppendJSONFloat appends a float64 in shortest round-trip decimal form.
func AppendJSONFloat(dst []byte, f float64) []byte {
	return strconv.AppendFloat(dst, f, 'f', -1, 64)
}

// AppendJSONInt appends a signed integer in decimal.
func AppendJSONInt(dst []byte, n int64) []byte {
	return strconv.AppendInt(dst, n, 10)
}

// AppendJSONUint appends an unsigned integer in decimal.
func AppendJSONUint(dst []byte, n uint64) []byte {
	return strconv.AppendUint(dst, n, 10)
}

// AppendJSONBool appends true or false.
func AppendJSONBool(dst []byte, v bool) []byte {
	if v {
		return append(dst, "true"...)
	}
	return append(dst, "false"...)
}
