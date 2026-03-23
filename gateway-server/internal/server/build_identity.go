package server

import (
	"strings"
	"sync/atomic"
)

var (
	buildVersion atomic.Value
	buildCommit  atomic.Value
	buildTime    atomic.Value
)

func init() {
	buildVersion.Store("dev")
	buildCommit.Store("unknown")
	buildTime.Store("unknown")
}

func SetBuildIdentity(version, commit, time string) {
	buildVersion.Store(firstNonEmpty(strings.TrimSpace(version), "dev"))
	buildCommit.Store(firstNonEmpty(strings.TrimSpace(commit), "unknown"))
	buildTime.Store(firstNonEmpty(strings.TrimSpace(time), "unknown"))
}
