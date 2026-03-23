package netutil

import "strings"

// IsAddrInUseError 统一判断跨平台端口占用错误文本，
// 避免启动链路和 RTP 链路各自维护一份字符串匹配规则。
func IsAddrInUseError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "address already in use") || strings.Contains(text, "only one usage of each socket address")
}
