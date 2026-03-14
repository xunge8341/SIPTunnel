package nodeconfig

import (
	"fmt"
	"strings"

	"siptunnel/internal/config"
)

type CheckResult struct {
	Level      string `json:"level"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	ActionHint string `json:"action_hint"`
}

type CompatibilityStatus struct {
	CurrentNetworkMode    config.NetworkMode `json:"current_network_mode"`
	CurrentCapability     config.Capability  `json:"current_capability"`
	LocalNodeConfigValid  bool               `json:"local_node_config_valid"`
	PeerNodeConfigValid   bool               `json:"peer_node_config_valid"`
	NetworkCompatibility  bool               `json:"network_mode_compatibility"`
	LocalNodeCheck        CheckResult        `json:"local_node_check"`
	PeerNodeCheck         CheckResult        `json:"peer_node_check"`
	CompatibilityCheck    CheckResult        `json:"compatibility_status"`
	IncompatiblePeerNodes []string           `json:"incompatible_peer_nodes,omitempty"`
}

func EvaluateCompatibility(local LocalNodeConfig, peers []PeerNodeConfig, currentMode config.NetworkMode, capability config.Capability) CompatibilityStatus {
	mode := currentMode.Normalize()
	status := CompatibilityStatus{CurrentNetworkMode: mode, CurrentCapability: capability}

	if err := local.Validate(); err != nil {
		status.LocalNodeConfigValid = false
		status.LocalNodeCheck = CheckResult{
			Level:      "error",
			Message:    fmt.Sprintf("本端 node 配置非法: %v", err),
			Suggestion: "修复 node_id/node_name/network_mode 与 SIP/RTP 必填字段后重试。",
			ActionHint: "更新本端节点配置后重新执行 /api/selfcheck。",
		}
	} else if local.NetworkMode.Normalize() != mode {
		status.LocalNodeConfigValid = false
		status.LocalNodeCheck = CheckResult{
			Level:      "error",
			Message:    fmt.Sprintf("本端 node.network_mode=%s 与当前 network_mode=%s 不一致", local.NetworkMode.Normalize(), mode),
			Suggestion: "将本端节点 network_mode 调整为当前运行模式，或先切换全局运行模式后再保存。",
			ActionHint: "统一 network_mode 后重启并复核节点详情与自检。",
		}
	} else {
		status.LocalNodeConfigValid = true
		status.LocalNodeCheck = CheckResult{Level: "info", Message: "本端 node 配置与当前 network_mode 一致", Suggestion: "无需处理。", ActionHint: "保持当前配置并纳入巡检基线。"}
	}

	status.PeerNodeConfigValid = true
	incompatiblePeers := make([]string, 0)
	for _, peer := range peers {
		if err := peer.Validate(); err != nil {
			status.PeerNodeConfigValid = false
			incompatiblePeers = append(incompatiblePeers, peer.PeerNodeID)
			continue
		}
		if !peer.Enabled {
			continue
		}
		if peer.SupportedNetworkMode.Normalize() != mode {
			status.PeerNodeConfigValid = false
			incompatiblePeers = append(incompatiblePeers, peer.PeerNodeID)
		}
	}
	if status.PeerNodeConfigValid {
		status.PeerNodeCheck = CheckResult{Level: "info", Message: "对端 peer 配置与当前 network_mode 兼容", Suggestion: "无需处理。", ActionHint: "保持当前 peer 配置并定期复核。"}
	} else {
		status.PeerNodeCheck = CheckResult{
			Level:      "error",
			Message:    fmt.Sprintf("存在与当前 network_mode=%s 不兼容的 peer 节点: %s", mode, strings.Join(incompatiblePeers, ",")),
			Suggestion: "修复 peer 关键字段并确保 supported_network_mode 与当前模式一致（可先禁用不兼容 peer）。",
			ActionHint: "逐个修复 peer 后重新保存并复核 /api/selfcheck。",
		}
	}
	status.IncompatiblePeerNodes = incompatiblePeers

	status.NetworkCompatibility = status.LocalNodeConfigValid && status.PeerNodeConfigValid
	if status.NetworkCompatibility {
		status.CompatibilityCheck = CheckResult{Level: "info", Message: "本端/对端节点配置与当前能力矩阵兼容", Suggestion: "无需处理。", ActionHint: "继续按照当前 network_mode 运行。"}
	} else {
		status.CompatibilityCheck = CheckResult{Level: "error", Message: "检测到 node/peer 与当前 network_mode 或 capability 不兼容", Suggestion: "优先修复本端 network_mode 一致性，再处理 peer 兼容性与缺失字段。", ActionHint: "修复后重新执行自检并导出诊断包确认。"}
	}

	return status
}
