package server

import "net"

// TuneTCPListener exposes the internal TCP accept-side tuning so the main
// gateway HTTP server can reuse the same listener settings as mapping runtimes.
func TuneTCPListener(ln net.Listener) net.Listener {
	return newTunedTCPListener(ln)
}
