package server

import (
	"net"
	"net/http"
	"os"
	"strings"
)

var defaultTrustedProxyCIDRs = []string{"127.0.0.0/8", "::1/128"}

func requestClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	trusted := trustedProxyNetworksFromEnv()
	if ip := forwardedClientIP(r, trusted); ip != "" {
		return ip
	}
	return remoteRequestIP(r)
}

func trustedProxyNetworksFromEnv() []*net.IPNet {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_TRUSTED_PROXY_CIDRS"))
	items := defaultTrustedProxyCIDRs
	if raw != "" {
		items = strings.Split(raw, ",")
	}
	out := make([]*net.IPNet, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			if _, network, err := net.ParseCIDR(item); err == nil {
				out = append(out, network)
			}
			continue
		}
		ip := net.ParseIP(item)
		if ip == nil {
			continue
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return out
}

func forwardedClientIP(r *http.Request, trusted []*net.IPNet) string {
	if r == nil {
		return ""
	}
	remoteIP := parseRemoteAddrIP(r.RemoteAddr)
	if remoteIP == nil || !ipInNetworks(remoteIP, trusted) {
		return ""
	}
	chain := parseForwardedIPChain(r)
	if len(chain) == 0 {
		return ""
	}
	for idx := len(chain) - 1; idx >= 0; idx-- {
		if !ipInNetworks(chain[idx], trusted) {
			return chain[idx].String()
		}
	}
	return chain[0].String()
}

func parseForwardedIPChain(r *http.Request) []net.IP {
	if r == nil {
		return nil
	}
	out := make([]net.IP, 0, 4)
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if host, _, err := net.SplitHostPort(raw); err == nil {
			raw = host
		}
		raw = strings.Trim(raw, "[]")
		if ip := net.ParseIP(raw); ip != nil {
			out = append(out, ip)
		}
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		for _, part := range strings.Split(forwarded, ",") {
			add(part)
		}
	}
	if len(out) == 0 {
		add(r.Header.Get("X-Real-IP"))
	}
	return out
}

func remoteRequestIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if ip := parseRemoteAddrIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func parseRemoteAddrIP(remoteAddr string) net.IP {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}
	remoteAddr = strings.Trim(remoteAddr, "[]")
	return net.ParseIP(remoteAddr)
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, network := range networks {
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}
