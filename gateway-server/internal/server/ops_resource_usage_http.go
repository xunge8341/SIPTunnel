package server

import (
	"context"
	"net/http"
	"runtime"
	"time"
)

func buildSystemResourceUsage(d *handlerDeps) systemResourceUsage {
	status := NodeNetworkStatus{}
	if d != nil && d.networkStatusFunc != nil {
		status = d.networkStatusFunc(context.Background())
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	activeRequests := 0
	if d != nil && d.protectionRuntime != nil {
		activeRequests = d.protectionRuntime.Snapshot().ActiveRequests
	}
	tuning := currentTransportTuning()
	converged := effectiveGenericDownloadConvergence()
	lastGC := ""
	if mem.LastGC > 0 {
		lastGC = time.Unix(0, int64(mem.LastGC)).UTC().Format(time.RFC3339)
	}
	usage := systemResourceUsage{
		CapturedAt:                          time.Now().UTC().Format(time.RFC3339),
		CPUCores:                            runtime.NumCPU(),
		GOMAXPROCS:                          runtime.GOMAXPROCS(0),
		Goroutines:                          runtime.NumGoroutine(),
		HeapAllocBytes:                      mem.HeapAlloc,
		HeapSysBytes:                        mem.HeapSys,
		HeapIdleBytes:                       mem.HeapIdle,
		StackInuseBytes:                     mem.StackInuse,
		LastGCTime:                          lastGC,
		SIPConnections:                      status.SIP.CurrentConnections,
		RTPOpenTransfers:                    status.RTP.ActiveTransfers,
		RTPPortPoolUsed:                     status.RTP.PortPoolUsed,
		RTPPortPoolTotal:                    status.RTP.PortPoolTotal,
		ActiveRequests:                      activeRequests,
		ConfiguredGenericDownloadMbps:       bpsToMbps(tuning.GenericDownloadTotalBitrateBps),
		ConfiguredGenericPerTransferMbps:    bpsToMbps(tuning.GenericDownloadMinPerTransferBitrateBps),
		ConfiguredAdaptiveHotCacheMB:        bytesToMB(tuning.AdaptivePlaybackSegmentCacheBytes),
		ConfiguredAdaptiveHotWindowMB:       bytesToMB(tuning.AdaptivePlaybackHotWindowBytes),
		ConfiguredGenericDownloadWindowMB:   bytesToMB(tuning.GenericDownloadWindowBytes),
		ConfiguredGenericSegmentConcurrency: tuning.GenericDownloadSegmentConcurrency,
		ConfiguredGenericRTPReorderWindow:   converged.ReorderWindowPackets,
		ConfiguredGenericRTPLossTolerance:   converged.LossTolerancePackets,
		ConfiguredGenericRTPGapTimeoutMS:    converged.GapTimeoutMS,
		ConfiguredGenericRTPFECEnabled:      converged.FECEnabled,
		ConfiguredGenericRTPFECGroupPackets: converged.FECGroupPackets,
		TheoreticalRTPTransferLimit:         maxIntVal(1, status.RTP.PortPoolTotal/2),
	}
	applySystemResourceAssessment(d, &usage, status)
	return usage
}

func bytesToMB(v int64) float64 { return float64(v) / (1024 * 1024) }
func bpsToMbps(v int) float64   { return float64(v) / (1024 * 1024) }

func (d *handlerDeps) handleSystemResourceUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: buildSystemResourceUsage(d)})
}
