package server

import (
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type gatewayRestartCommand struct {
	Executable string
	Args       []string
	Summary    string
}

func resolveGatewayRestartCommand() (gatewayRestartCommand, error) {
	if raw := strings.TrimSpace(os.Getenv("SIPTUNNEL_RESTART_COMMAND")); raw != "" {
		if runtime.GOOS == "windows" {
			return gatewayRestartCommand{Executable: "powershell", Args: []string{"-NoProfile", "-Command", "Start-Sleep -Milliseconds 800; " + raw}, Summary: raw}, nil
		}
		return gatewayRestartCommand{Executable: "sh", Args: []string{"-c", "sleep 1; " + raw}, Summary: raw}, nil
	}
	if runtime.GOOS == "windows" {
		return gatewayRestartCommand{Executable: "powershell", Args: []string{"-NoProfile", "-Command", "Start-Sleep -Milliseconds 800; Restart-Service -Name SIPTunnelGateway -Force"}, Summary: "Restart-Service -Name SIPTunnelGateway -Force"}, nil
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		return gatewayRestartCommand{Executable: "sh", Args: []string{"-c", "sleep 1; systemctl restart siptunnel-gateway.service"}, Summary: "systemctl restart siptunnel-gateway.service"}, nil
	}
	return gatewayRestartCommand{}, exec.ErrNotFound
}

func (d *handlerDeps) handleGatewayRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	cmd, err := resolveGatewayRestartCommand()
	if err != nil {
		writeError(w, http.StatusBadRequest, "RESTART_NOT_SUPPORTED", "未检测到可用重启命令，请配置环境变量 SIPTUNNEL_RESTART_COMMAND")
		return
	}
	go func() {
		command := exec.Command(cmd.Executable, cmd.Args...)
		_ = command.Run()
	}()
	d.recordOpsAudit(r, readOperator(r), "RESTART_GATEWAY", map[string]any{"command": cmd.Summary})
	writeJSON(w, http.StatusAccepted, responseEnvelope{Code: "OK", Message: "accepted", Data: map[string]any{
		"accepted":     true,
		"command":      cmd.Summary,
		"scheduled_at": formatTimestamp(time.Now().UTC()),
	}})
}
