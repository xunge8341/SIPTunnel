package selfcheck

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type portOwnerInfo struct {
	Summary   string
	PID       string
	SelfOwned bool
}

func (r *Runner) detectPortOwnerInfo(port int) portOwnerInfo {
	if r == nil || r.lookPath == nil || r.execCommand == nil || port <= 0 {
		return portOwnerInfo{}
	}
	ctx := context.Background()
	if runnerGOOS(r) == "windows" {
		pid := r.detectWindowsPID(ctx, port)
		if pid == "" {
			return portOwnerInfo{}
		}
		info := portOwnerInfo{PID: pid, SelfOwned: pid == strconv.Itoa(os.Getpid())}
		info.Summary = r.lookupWindowsProcess(ctx, pid)
		if info.Summary == "" {
			info.Summary = "pid=" + pid
		}
		if info.SelfOwned && info.Summary == "" {
			info.Summary = fmt.Sprintf("current-process(pid=%s)", pid)
		}
		return info
	}
	return r.lookupLinuxProcess(ctx, port)
}

func (r *Runner) lookupLinuxProcess(ctx context.Context, port int) portOwnerInfo {
	if _, err := r.lookPath("lsof"); err != nil {
		return portOwnerInfo{}
	}
	out, err := r.execCommand(ctx, "lsof", "-nP", fmt.Sprintf("-i:%d", port))
	if err != nil {
		return portOwnerInfo{}
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid := fields[1]
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		return portOwnerInfo{
			Summary:   fields[0] + "(pid=" + pid + ")",
			PID:       pid,
			SelfOwned: pid == strconv.Itoa(os.Getpid()),
		}
	}
	return portOwnerInfo{}
}

func (r *Runner) detectWindowsPID(ctx context.Context, port int) string {
	if _, err := r.lookPath("netstat"); err != nil {
		return ""
	}
	out, err := r.execCommand(ctx, "netstat", "-ano")
	if err != nil {
		return ""
	}
	target := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, target) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid := fields[len(fields)-1]
		if _, err := strconv.Atoi(pid); err == nil {
			return pid
		}
	}
	return ""
}

func (r *Runner) lookupWindowsProcess(ctx context.Context, pid string) string {
	if pid == "" {
		return ""
	}
	if _, err := r.lookPath("tasklist"); err != nil {
		return "pid=" + pid
	}
	out, err := r.execCommand(ctx, "tasklist", "/fi", "PID eq "+pid)
	if err != nil {
		return "pid=" + pid
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[len(fields)-1] == pid {
			return fields[0] + "(pid=" + pid + ")"
		}
	}
	return "pid=" + pid
}
