package binance

import "net"

// ipv4Dialer forces all connections to use IPv4 (tcp4).
// This avoids "Unmatched IP" errors when API keys are whitelisted
// by IPv4 address but the default dialer resolves to IPv6.
type ipv4Dialer struct{}

func (d *ipv4Dialer) Dial(_, addr string) (net.Conn, error) {
	return net.DialTimeout("tcp4", addr, 5_000_000_000) // 5s
}
