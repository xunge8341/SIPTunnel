package server

import (
	"net"
	"strings"
	"sync"
)

var udpAddrCache sync.Map

func cachedResolveUDPAddr(address string) (*net.UDPAddr, error) {
	key := strings.TrimSpace(address)
	if key == "" {
		return nil, net.InvalidAddrError("empty udp address")
	}
	if existing, ok := udpAddrCache.Load(key); ok {
		return cloneUDPAddr(existing.(*net.UDPAddr)), nil
	}
	resolved, err := net.ResolveUDPAddr("udp", key)
	if err != nil {
		return nil, err
	}
	udpAddrCache.Store(key, cloneUDPAddr(resolved))
	return cloneUDPAddr(resolved), nil
}

func cloneUDPAddr(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	cp := *addr
	if addr.IP != nil {
		cp.IP = append(net.IP(nil), addr.IP...)
	}
	if addr.Zone != "" {
		cp.Zone = addr.Zone
	}
	return &cp
}
