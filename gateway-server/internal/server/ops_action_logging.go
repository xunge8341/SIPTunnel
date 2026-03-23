package server

import (
	"log"
	"strings"
)

func logTunnelCatalogAction(action string, subscribeTriggered, notifyTriggered int, snapshot GB28181Snapshot) {
	log.Printf("tunnel-catalog action=%s subscribe_triggered=%d notify_triggered=%d peers=%d pending=%d inbound=%d catalog_resources=%d catalog_exposed=%d", strings.TrimSpace(action), subscribeTriggered, notifyTriggered, len(snapshot.Peers), len(snapshot.Pending), len(snapshot.Inbound), snapshot.Catalog.ResourceTotal, snapshot.Catalog.ExposedTotal)
}

func logTunnelSessionAction(action string, state tunnelSessionRuntimeState) {
	log.Printf("tunnel-session action=%s registration_status=%s heartbeat_status=%s last_failure=%s next_retry=%s", strings.TrimSpace(action), strings.TrimSpace(state.RegistrationStatus), strings.TrimSpace(state.HeartbeatStatus), strings.TrimSpace(state.LastFailureReason), strings.TrimSpace(state.NextRetryTime))
}

func logMappingTestResult(result MappingTestResponse) {
	log.Printf("mapping-test status=%s passed=%t failure_stage=%s failure_reason=%s signaling_request=%s response_channel=%s registration_status=%s", strings.TrimSpace(result.Status), result.Passed, strings.TrimSpace(result.FailureStage), strings.TrimSpace(result.FailureReason), strings.TrimSpace(result.SignalingRequest), strings.TrimSpace(result.ResponseChannel), strings.TrimSpace(result.RegistrationStatus))
}

func logLocalResourceAction(action string, item LocalResourceRecord) {
	log.Printf("local-resource action=%s resource_code=%s enabled=%t target_url=%s methods=%s response_mode=%s updated_at=%s", strings.TrimSpace(action), strings.TrimSpace(item.ResourceCode), item.Enabled, strings.TrimSpace(item.TargetURL), strings.Join(item.Methods, ","), strings.TrimSpace(item.ResponseMode), strings.TrimSpace(item.UpdatedAt))
}

func logWorkspaceApplied(req nodeTunnelWorkspace) {
	log.Printf("node-tunnel workspace_applied local_device_id=%s local_ip=%s sip_port=%d rtp_range=%d-%d mapping_range=%d-%d peer_device_id=%s peer_ip=%s peer_sip_port=%d network_mode=%s sip_transport=%s rtp_transport=%s relay_mode=%s", strings.TrimSpace(req.LocalNode.DeviceID), strings.TrimSpace(req.LocalNode.NodeIP), req.LocalNode.SignalingPort, req.LocalNode.RTPPortStart, req.LocalNode.RTPPortEnd, req.LocalNode.MappingPortStart, req.LocalNode.MappingPortEnd, strings.TrimSpace(req.PeerNode.DeviceID), strings.TrimSpace(req.PeerNode.NodeIP), req.PeerNode.SignalingPort, strings.TrimSpace(req.NetworkMode), strings.TrimSpace(asString(req.SIPCapability["transport"])), strings.TrimSpace(asString(req.RTPCapability["transport"])), strings.TrimSpace(req.SessionSettings.MappingRelayMode))
}
