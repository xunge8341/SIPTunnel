package server

import (
	"errors"
	"net"
	"strings"
)

func shouldRetrySIPTCPSend(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	lowered := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, token := range []string{"broken pipe", "connection reset", "forcibly closed", "use of closed network connection", "eof"} {
		if strings.Contains(lowered, token) {
			return true
		}
	}
	return false
}
