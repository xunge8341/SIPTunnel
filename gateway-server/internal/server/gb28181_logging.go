package server

import (
	"log"
	"os"
	"strings"
)

func logGB28181Successf(format string, args ...any) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("GATEWAY_GB28181_VERBOSE_LOG")), "true") {
		return
	}
	log.Printf(format, args...)
}
