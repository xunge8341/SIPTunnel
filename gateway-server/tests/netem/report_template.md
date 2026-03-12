# SIPTunnel 网络劣化测试报告

- 生成时间：{{ now }}
- 测试对象：SIP TCP / RTP UDP / RTP TCP

## 结果总览

| 链路 | 场景 | 条件 | 成功率 | 平均时延(ms) | 重传率 | 恢复时间(ms) |
|---|---|---|---:|---:|---:|---:|
{{- range .Summaries }}
| {{ .Link }} | {{ .Scenario }} | delay={{.Condition.DelayMS}}ms jitter={{.Condition.JitterMS}}ms loss={{.Condition.LossPercent}}% reorder={{.Condition.ReorderPercent}}% disconnect={{.Condition.DisconnectMS}}ms bw={{.Condition.BandwidthKbps}}kbps | {{ percent .SuccessRate }} | {{ printf "%.2f" .AvgLatencyMS }} | {{ percent .RetransmitRate }} | {{ printf "%.2f" .RecoveryTimeMS }} |
{{- end }}

## 手动验证记录（用于不可自动化场景）
{{- range .Summaries }}
### {{ .Link }} / {{ .Scenario }}
{{- if .ManualValidation }}
{{- range .ManualValidation }}
- [ ] {{ . }}
{{- end }}
{{- else }}
- 无
{{- end }}
{{ end }}
