<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header
      title="系统设置"
      sub-title="维护持久化、清理计划与保留策略；UI 默认为内嵌模式。"
    >
      <template #extra>
        <a-space>
          <a-button @click="load">刷新回读</a-button>
          <a-button type="primary" :loading="saving" @click="save"
            >保存设置</a-button
          >
        </a-space>
      </template>
    </a-page-header>

    <a-alert
      type="info"
      show-icon
      message="当前页字段均可直接编辑，保存后会立即重新回读。"
    />
    <a-alert v-if="notice" type="success" :message="notice" show-icon />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-alert
      v-if="validationErrors.length"
      type="warning"
      show-icon
      message="请先修正以下系统设置问题"
    >
      <template #description
        ><ul class="validation-list">
          <li v-for="item in validationErrors" :key="item">{{ item }}</li>
        </ul></template
      >
    </a-alert>

    <a-spin :spinning="loading || saving">
      <a-empty v-if="!form" description="暂无系统设置" />
      <template v-else>
        <a-row :gutter="[16, 16]" align="top">
          <a-col :xs="24" :xl="12">
            <a-card class="full-card" title="持久化与数据库" :bordered="false">
              <a-form layout="vertical">
                <a-form-item
                  label="SQLite 路径"
                  extra="访问日志、审计和清理状态都会写入 SQLite。"
                >
                  <a-input v-model:value="form.sqlitePath" />
                </a-form-item>
                <a-form-item label="UI 承载模式"
                  ><a-input :value="uiModeLabel" disabled
                /></a-form-item>
                <a-row :gutter="12">
                  <a-col :span="12"
                    ><a-form-item label="任务保留天数"
                      ><a-input-number
                        v-model:value="form.logRetentionDays"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :span="12"
                    ><a-form-item label="任务保留条数"
                      ><a-input-number
                        v-model:value="form.logRetentionRecords"
                        :min="100"
                        :step="100"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                </a-row>
              </a-form>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="12">
            <a-card class="full-card" title="日志与清理" :bordered="false">
              <a-form layout="vertical">
                <a-form-item
                  label="清理计划（Cron）"
                  extra="用于控制数据库和日志清理节奏。"
                  ><a-input v-model:value="form.cleanupCron"
                /></a-form-item>
                <a-row :gutter="12">
                  <a-col :span="12"
                    ><a-form-item label="访问日志保留天数"
                      ><a-input-number
                        v-model:value="form.accessLogRetentionDays"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :span="12"
                    ><a-form-item label="访问日志保留条数"
                      ><a-input-number
                        v-model:value="form.accessLogRetentionRecords"
                        :min="100"
                        :step="100"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="12"
                    ><a-form-item label="审计保留天数"
                      ><a-input-number
                        v-model:value="form.auditRetentionDays"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :span="12"
                    ><a-form-item label="审计保留条数"
                      ><a-input-number
                        v-model:value="form.auditRetentionRecords"
                        :min="100"
                        :step="100"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                </a-row>
              </a-form>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]" align="top">
          <a-col :xs="24" :xl="12">
            <a-card class="full-card" title="保留策略" :bordered="false">
              <a-form layout="vertical">
                <a-row :gutter="12">
                  <a-col :span="12"
                    ><a-form-item label="诊断保留天数"
                      ><a-input-number
                        v-model:value="form.diagnosticsRetentionDays"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :span="12"
                    ><a-form-item label="诊断保留条数"
                      ><a-input-number
                        v-model:value="form.diagnosticsRetentionRecords"
                        :min="10"
                        :step="10"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="12"
                    ><a-form-item label="压测保留天数"
                      ><a-input-number
                        v-model:value="form.loadtestRetentionDays"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :span="12"
                    ><a-form-item label="压测保留条数"
                      ><a-input-number
                        v-model:value="form.loadtestRetentionRecords"
                        :min="10"
                        :step="10"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                </a-row>
                <a-alert
                  type="info"
                  show-icon
                  message="保留策略修改后会在下一次清理周期生效。"
                />
              </a-form>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="12">
            <a-card class="full-card" title="最近维护动作" :bordered="false">
              <a-form layout="vertical">
                <a-form-item label="最近清理结果"
                  ><a-textarea
                    :value="cleanupSummary"
                    :auto-size="{ minRows: 3, maxRows: 5 }"
                    disabled
                /></a-form-item>
              </a-form>
            </a-card>
          </a-col>
        </a-row>

        <a-card class="full-card" title="观测与探针入口" :bordered="false">
          <a-alert
            type="info"
            show-icon
            message="P1 阶段已补齐 /metrics 探针；建议同时把 /readyz、/api/selfcheck 和 /api/startup-summary 纳入排障链路。"
            style="margin-bottom: 16px"
          />
          <a-descriptions :column="1" size="small">
            <a-descriptions-item label="API 基地址"
              ><span class="mono-inline-code wrap-break-anywhere">{{
                form.apiBaseUrl
              }}</span></a-descriptions-item
            >
            <a-descriptions-item label="Metrics"
              ><span class="mono-inline-code wrap-break-anywhere">{{
                form.metricsEndpoint
              }}</span></a-descriptions-item
            >
            <a-descriptions-item label="Ready 探针"
              ><span class="mono-inline-code wrap-break-anywhere">{{
                form.readyEndpoint
              }}</span></a-descriptions-item
            >
            <a-descriptions-item label="Self-check"
              ><span class="mono-inline-code wrap-break-anywhere">{{
                form.selfCheckEndpoint
              }}</span></a-descriptions-item
            >
            <a-descriptions-item label="启动摘要"
              ><span class="mono-inline-code wrap-break-anywhere">{{
                form.startupSummaryEndpoint
              }}</span></a-descriptions-item
            >
          </a-descriptions>
        </a-card>

        <a-row :gutter="[16, 16]" align="top">
          <a-col :xs="24" :xl="12">
            <a-card
              class="full-card"
              title="源码 / 构建 / 发布物一致性"
              :bordered="false"
            >
              <a-space direction="vertical" style="width: 100%" size="middle">
                <a-alert
                  type="info"
                  show-icon
                  message="用于运维快速判断当前运行态 UI 是否仍与源码和构建产物一致，避免‘源码已改、发布物未同步’。"
                />
                <a-descriptions :column="1" size="small">
                  <a-descriptions-item label="一致性状态"
                    ><a-tag :color="uiConsistencyTagColor">{{
                      form.uiConsistencyStatus || 'unknown'
                    }}</a-tag></a-descriptions-item
                  >
                  <a-descriptions-item label="一致性说明">{{
                    form.uiConsistencyDetail || '-'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="构建批次 Nonce"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.uiEmbedBuildNonce || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="嵌入时间"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.uiEmbeddedAt || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="源码最近修改"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.uiSourceLatestWrite || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="嵌入包 Hash"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.uiEmbeddedHash || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="资源基路径模式"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.uiAssetBaseMode || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="Router BasePath 策略"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.uiRouterBasePathPolicy || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="源码清洁守卫状态"
                    ><a-tag :color="uiDeliveryGuardTagColor">{{
                      form.uiDeliveryGuardStatus || 'unknown'
                    }}</a-tag></a-descriptions-item
                  >
                  <a-descriptions-item label="源码清洁守卫说明">{{
                    form.uiDeliveryGuardDetail || '-'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="守卫清理遗留文件数"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.uiDeliveryGuardRemovedCount ?? 0
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="剩余遗留文件 / 活跃命中"
                    ><span class="mono-inline-code wrap-break-anywhere"
                      >{{ form.uiDeliveryGuardRemainingCount ?? 0 }} /
                      {{ form.uiDeliveryGuardHitCount ?? 0 }}</span
                    ></a-descriptions-item
                  >
                </a-descriptions>
              </a-space>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="12">
            <a-card
              class="full-card"
              title="入口与控制面活跃策略"
              :bordered="false"
            >
              <a-space direction="vertical" style="width: 100%" size="middle">
                <a-alert
                  type="info"
                  show-icon
                  message="运维排障时直接查看当前入口选择、UDP 控制面预算救援、以及大响应 RTP 容忍策略，避免多份文档口径打架。"
                />
                <a-descriptions :column="1" size="small">
                  <a-descriptions-item label="入口选择策略"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.entrySelectionPolicy || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="UDP 控制面头策略"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.udpControlHeaderPolicy || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="大文件 / 视频 RTP 容忍"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.genericDownloadRTPToleranceProfile || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="大文件 RTP 收口守卫"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      form.genericDownloadGuardPolicy || '-'
                    }}</span></a-descriptions-item
                  >
                </a-descriptions>
              </a-space>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]" align="top">
          <a-col :xs="24" :xl="24">
            <a-card
              class="full-card"
              title="运行时收口参数（可保存，统一 MB / Mbps）"
              :bordered="false"
            >
              <a-space direction="vertical" style="width: 100%" size="middle">
                <a-alert
                  type="info"
                  show-icon
                  message="这里直接配置运行时大文件 / 热点窗口收口参数，页面统一按 MB / Mbps 展示，避免继续让运维填写巨大的 bps/bytes 原始值。"
                />
                <a-form layout="vertical">
                  <a-row :gutter="12">
                    <a-col :xs="24" :md="8"
                      ><a-form-item label="大文件总带宽上限（Mbps）"
                        ><a-input-number
                          v-model:value="form.genericDownloadTotalMbps"
                          :min="1"
                          :step="1"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                    <a-col :xs="24" :md="8"
                      ><a-form-item label="单传输保底带宽（Mbps）"
                        ><a-input-number
                          v-model:value="form.genericDownloadPerTransferMbps"
                          :min="0.5"
                          :step="0.5"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                    <a-col :xs="24" :md="8"
                      ><a-form-item label="大文件窗口（MB）"
                        ><a-input-number
                          v-model:value="form.genericDownloadWindowMB"
                          :min="0.5"
                          :step="0.5"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                  </a-row>
                  <a-row :gutter="12">
                    <a-col :xs="24" :md="8"
                      ><a-form-item label="热点缓存（MB）"
                        ><a-input-number
                          v-model:value="form.adaptiveHotCacheMB"
                          :min="1"
                          :step="1"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                    <a-col :xs="24" :md="8"
                      ><a-form-item label="热点窗口（MB）"
                        ><a-input-number
                          v-model:value="form.adaptiveHotWindowMB"
                          :min="1"
                          :step="1"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                    <a-col :xs="24" :md="8"
                      ><a-form-item label="大文件段并发"
                        ><a-input-number
                          v-model:value="form.genericDownloadSegmentConcurrency"
                          :min="1"
                          :step="1"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                  </a-row>
                  <a-row :gutter="12">
                    <a-col :xs="24" :md="6"
                      ><a-form-item label="乱序窗口（包）"
                        ><a-input-number
                          v-model:value="
                            form.genericDownloadRTPReorderWindowPackets
                          "
                          :min="32"
                          :step="32"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                    <a-col :xs="24" :md="6"
                      ><a-form-item label="丢包容忍（包）"
                        ><a-input-number
                          v-model:value="
                            form.genericDownloadRTPLossTolerancePackets
                          "
                          :min="1"
                          :step="1"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                    <a-col :xs="24" :md="6"
                      ><a-form-item label="Gap 超时（ms）"
                        ><a-input-number
                          v-model:value="form.genericDownloadRTPGapTimeoutMS"
                          :min="100"
                          :step="100"
                          style="width: 100%" /></a-form-item
                    ></a-col>
                    <a-col :xs="24" :md="6"
                      ><a-form-item label="FEC 组大小（包）"
                        ><a-input-number
                          v-model:value="form.genericDownloadRTPFECGroupPackets"
                          :min="2"
                          :step="1"
                          style="width: 100%"
                          :disabled="
                            !form.genericDownloadRTPFECEnabled
                          " /></a-form-item
                    ></a-col>
                  </a-row>
                  <a-row :gutter="12">
                    <a-col :xs="24" :md="8"
                      ><a-form-item label="启用大文件 RTP FEC"
                        ><a-switch
                          v-model:checked="
                            form.genericDownloadRTPFECEnabled
                          " /></a-form-item
                    ></a-col>
                  </a-row>
                </a-form>
              </a-space>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]" align="top">
          <a-col :xs="24" :xl="12">
            <a-card
              class="full-card"
              title="系统资源与收口参数（MB / Mbps）"
              :bordered="false"
            >
              <a-space direction="vertical" style="width: 100%" size="middle">
                <a-alert
                  type="info"
                  show-icon
                  message="运维侧统一查看 CPU、内存、活跃连接，以及当前生效的大文件带宽、热点缓存与热点窗口，统一采用 MB / Mbps 口径。"
                />
                <a-empty v-if="!resourceUsage" description="暂无资源视图" />
                <a-descriptions v-else :column="1" size="small">
                  <a-descriptions-item label="采集时间"
                    ><span class="mono-inline-code wrap-break-anywhere">{{
                      resourceUsage.captured_at || '-'
                    }}</span></a-descriptions-item
                  >
                  <a-descriptions-item label="运行结论"
                    ><a-tag
                      :color="
                        resourceStatusTagColor(resourceUsage.status_color)
                      "
                      >{{ resourceUsage.status_summary || '未评估' }}</a-tag
                    ></a-descriptions-item
                  >
                  <a-descriptions-item label="推荐档位">{{
                    resourceUsage.recommended_profile || '平衡模式'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="运行时已应用档位"
                    >{{
                      resourceUsage.runtime_profile_applied ||
                      resourceUsage.recommended_profile ||
                      '平衡模式'
                    }}<span v-if="resourceUsage.runtime_profile_changed"
                      >（本轮已切换）</span
                    ></a-descriptions-item
                  >
                  <a-descriptions-item label="自检总体">{{
                    resourceUsage.selfcheck_overall || '未获取'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="CPU / GOMAXPROCS"
                    >{{ resourceUsage.cpu_cores }} /
                    {{ resourceUsage.gomaxprocs }}</a-descriptions-item
                  >
                  <a-descriptions-item label="Goroutines">{{
                    resourceUsage.goroutines
                  }}</a-descriptions-item>
                  <a-descriptions-item label="堆内存 / Sys"
                    >{{ formatMiB(resourceUsage.heap_alloc_bytes) }} /
                    {{
                      formatMiB(resourceUsage.heap_sys_bytes)
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="堆空闲 / 栈内存"
                    >{{ formatMiB(resourceUsage.heap_idle_bytes) }} /
                    {{
                      formatMiB(resourceUsage.stack_inuse_bytes)
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="当前活跃请求"
                    >{{ resourceUsage.active_requests }}（{{
                      resourceUsage.active_request_usage_percent ?? 0
                    }}%）</a-descriptions-item
                  >
                  <a-descriptions-item label="SIP 当前连接">{{
                    resourceUsage.sip_connections
                  }}</a-descriptions-item>
                  <a-descriptions-item label="RTP 活跃传输">{{
                    resourceUsage.rtp_active_transfers
                  }}</a-descriptions-item>
                  <a-descriptions-item label="RTP 端口池"
                    >{{ resourceUsage.rtp_port_pool_used }} /
                    {{ resourceUsage.rtp_port_pool_total }}（{{
                      resourceUsage.rtp_port_pool_usage_percent ?? 0
                    }}%）</a-descriptions-item
                  >
                  <a-descriptions-item label="理论稳定 RTP 并发">{{
                    resourceUsage.theoretical_rtp_transfer_limit ?? 0
                  }}</a-descriptions-item>
                  <a-descriptions-item label="建议文件并发 / 建议总并发"
                    >{{
                      resourceUsage.recommended_file_transfer_max_concurrent ??
                      0
                    }}
                    /
                    {{
                      resourceUsage.recommended_max_concurrent ?? 0
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="建议限流 RPS / Burst"
                    >{{ resourceUsage.recommended_rate_limit_rps ?? 0 }} /
                    {{
                      resourceUsage.recommended_rate_limit_burst ?? 0
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="大文件总带宽上限">{{
                    formatMbps(resourceUsage.configured_generic_download_mbps)
                  }}</a-descriptions-item>
                  <a-descriptions-item label="单传输保底带宽">{{
                    formatMbps(
                      resourceUsage.configured_generic_per_transfer_mbps
                    )
                  }}</a-descriptions-item>
                  <a-descriptions-item label="大文件窗口 / 段并发"
                    >{{
                      formatMBValue(
                        resourceUsage.configured_generic_download_window_mb
                      )
                    }}
                    /
                    {{
                      resourceUsage.configured_generic_segment_concurrency
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="热点缓存 / 热点窗口"
                    >{{
                      formatMBValue(
                        resourceUsage.configured_adaptive_hot_cache_mb
                      )
                    }}
                    /
                    {{
                      formatMBValue(
                        resourceUsage.configured_adaptive_hot_window_mb
                      )
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="RTP 乱序 / 容忍 / Gap"
                    >{{
                      resourceUsage.configured_generic_rtp_reorder_window_packets
                    }}
                    /
                    {{
                      resourceUsage.configured_generic_rtp_loss_tolerance_packets
                    }}
                    /
                    {{
                      resourceUsage.configured_generic_rtp_gap_timeout_ms
                    }}
                    ms</a-descriptions-item
                  >
                  <a-descriptions-item label="RTP FEC"
                    >{{
                      resourceUsage.configured_generic_rtp_fec_enabled
                        ? '已启用'
                        : '未启用'
                    }}
                    · 组大小
                    {{
                      resourceUsage.configured_generic_rtp_fec_group_packets
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="RTP jitter/loss / pending"
                    >{{ resourceUsage.observed_jitter_loss_events ?? 0 }} /
                    {{ resourceUsage.observed_gap_timeouts ?? 0 }} /
                    {{
                      resourceUsage.observed_peak_pending ?? 0
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="writer block / circuit open"
                    >{{ resourceUsage.observed_max_writer_block_ms ?? 0 }} ms /
                    {{
                      resourceUsage.observed_circuit_open_count ?? 0
                    }}</a-descriptions-item
                  >
                </a-descriptions>
              </a-space>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="12">
            <a-card class="full-card" title="运维建议" :bordered="false">
              <a-space direction="vertical" style="width: 100%" size="middle">
                <a-alert
                  type="warning"
                  show-icon
                  message="大文件、热点资源和热点来源 IP 的限流/临时限制建议在“告警与保护”页统一操作；系统设置页主要负责一致性与全局收口检查。"
                />
                <a-descriptions :column="1" size="small">
                  <a-descriptions-item label="推荐摘要">{{
                    resourceUsage?.recommended_summary ||
                    '优先以本页“一致性状态”“源码清洁守卫”“入口与控制面活跃策略”为准。'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="适合场景">{{
                    resourceUsage?.suitable_scenarios?.join('、') ||
                    '视频播放、大文件下载、混合场景'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="运行时已应用档位">{{
                    resourceUsage?.runtime_profile_applied ||
                    resourceUsage?.recommended_profile ||
                    '平衡模式'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="运行时写回时间">{{
                    resourceUsage?.runtime_profile_applied_at || '-'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="运行时写回说明">{{
                    resourceUsage?.runtime_profile_reason || '未写回'
                  }}</a-descriptions-item>
                  <a-descriptions-item label="建议动作">{{
                    resourceUsage?.suggested_actions?.join('；') ||
                    '热点来源 IP、热点映射资源建议先做 10~30 分钟临时限制，再观察命中和恢复情况。'
                  }}</a-descriptions-item>
                </a-descriptions>
              </a-space>
            </a-card>
          </a-col>
        </a-row>

        <a-card
          class="full-card"
          title="管理会话（仅浏览器本地）"
          :bordered="false"
        >
          <a-alert
            type="info"
            show-icon
            message="仅保存在当前浏览器本地，用于专网管理面令牌/MFA 头透传；不会写入后端配置文件。"
            style="margin-bottom: 16px"
          />
          <a-form layout="vertical">
            <a-row :gutter="12">
              <a-col :xs="24" :xl="8"
                ><a-form-item label="操作人"
                  ><a-input
                    v-model:value="adminSession.operator"
                    placeholder="例如 ops-admin" /></a-form-item
              ></a-col>
              <a-col :xs="24" :xl="8"
                ><a-form-item label="管理令牌"
                  ><a-input-password
                    v-model:value="adminSession.token"
                    placeholder="GATEWAY_ADMIN_TOKEN" /></a-form-item
              ></a-col>
              <a-col :xs="24" :xl="8"
                ><a-form-item label="MFA 口令"
                  ><a-input-password
                    v-model:value="adminSession.mfa"
                    placeholder="GATEWAY_ADMIN_MFA_CODE" /></a-form-item
              ></a-col>
            </a-row>
            <a-space>
              <a-button @click="saveAdminSession">保存到浏览器</a-button>
              <a-button danger ghost @click="clearAdminSession"
                >清空本地会话</a-button
              >
            </a-space>
            <a-descriptions style="margin-top: 16px" :column="1" size="small">
              <a-descriptions-item label="管理令牌已配置"
                ><a-tag
                  :color="form?.adminTokenConfigured ? 'green' : 'orange'"
                  >{{ form?.adminTokenConfigured ? '已启用' : '未启用' }}</a-tag
                ></a-descriptions-item
              >
              <a-descriptions-item label="MFA 已配置"
                ><a-tag
                  :color="form?.adminMfaConfigured ? 'green' : 'orange'"
                  >{{ form?.adminMfaConfigured ? '已配置' : '未配置' }}</a-tag
                ></a-descriptions-item
              >
              <a-descriptions-item label="配置落盘加密"
                ><a-tag
                  :color="form?.configEncryptionEnabled ? 'green' : 'orange'"
                  >{{
                    form?.configEncryptionEnabled ? '已启用' : '未启用'
                  }}</a-tag
                ></a-descriptions-item
              >
              <a-descriptions-item label="隧道签名密钥"
                ><a-tag
                  :color="form?.tunnelSignerExternalized ? 'green' : 'orange'"
                  >{{
                    form?.tunnelSignerExternalized ? '已外置' : '仍为默认值'
                  }}</a-tag
                ></a-descriptions-item
              >
            </a-descriptions>
          </a-form>
        </a-card>
      </template>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { SystemResourceUsage, SystemSettingsState } from '../types/gateway'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const form = ref<SystemSettingsState>()
const resourceUsage = ref<SystemResourceUsage>()
const validationErrors = ref<string[]>([])
const adminSession = ref({ operator: '', token: '', mfa: '' })

const uiModeLabel = computed(() =>
  form.value?.uiMode === 'embedded' ? '内嵌模式（默认）' : '外部模式'
)
const uiConsistencyTagColor = computed(() => {
  if (form.value?.uiConsistencyStatus === 'aligned') return 'green'
  if (form.value?.uiConsistencyStatus === 'degraded') return 'orange'
  return 'default'
})
const uiDeliveryGuardTagColor = computed(() => {
  if (form.value?.uiDeliveryGuardStatus === 'aligned') return 'green'
  if (form.value?.uiDeliveryGuardStatus === 'degraded') return 'orange'
  return 'default'
})

const cronPattern =
  /^(@(hourly|daily|weekly|monthly|yearly)|([^\s]+\s+){4}[^\s]+)$/
const validateSettings = () => {
  const issues: string[] = []
  const data = form.value
  if (!data) return ['系统设置尚未加载完成']
  if (!String(data.sqlitePath || '').trim()) issues.push('SQLite 路径不能为空')
  if (!cronPattern.test(String(data.cleanupCron || '').trim()))
    issues.push('清理计划（Cron）格式不正确')
  const positiveInt = (label: string, value?: number, min = 1) => {
    if (!value || value < min) issues.push(`${label}必须大于等于 ${min}`)
  }
  positiveInt('任务保留天数', data.logRetentionDays)
  positiveInt('任务保留条数', data.logRetentionRecords, 100)
  positiveInt('访问日志保留天数', data.accessLogRetentionDays)
  positiveInt('访问日志保留条数', data.accessLogRetentionRecords, 100)
  positiveInt('审计保留天数', data.auditRetentionDays)
  positiveInt('审计保留条数', data.auditRetentionRecords, 100)
  positiveInt('诊断保留天数', data.diagnosticsRetentionDays)
  positiveInt('诊断保留条数', data.diagnosticsRetentionRecords, 10)
  positiveInt('压测保留天数', data.loadtestRetentionDays)
  positiveInt('压测保留条数', data.loadtestRetentionRecords, 10)
  const positiveFloat = (label: string, value?: number, min = 0.1) => {
    if (!value || value < min) issues.push(`${label}必须大于等于 ${min}`)
  }
  positiveFloat('大文件总带宽上限（Mbps）', data.genericDownloadTotalMbps, 1)
  positiveFloat(
    '单传输保底带宽（Mbps）',
    data.genericDownloadPerTransferMbps,
    0.5
  )
  positiveFloat('大文件窗口（MB）', data.genericDownloadWindowMB, 0.5)
  positiveFloat('热点缓存（MB）', data.adaptiveHotCacheMB, 1)
  positiveFloat('热点窗口（MB）', data.adaptiveHotWindowMB, 1)
  positiveInt('大文件段并发', data.genericDownloadSegmentConcurrency, 1)
  positiveInt(
    'RTP 乱序窗口（包）',
    data.genericDownloadRTPReorderWindowPackets,
    32
  )
  positiveInt(
    'RTP 丢包容忍（包）',
    data.genericDownloadRTPLossTolerancePackets,
    1
  )
  positiveInt('RTP Gap 超时（ms）', data.genericDownloadRTPGapTimeoutMS, 100)
  if (data.genericDownloadRTPFECEnabled)
    positiveInt(
      'RTP FEC 组大小（包）',
      data.genericDownloadRTPFECGroupPackets,
      2
    )
  validationErrors.value = issues
  return issues
}

const cleanupSummary = computed(() => {
  if (!form.value) return '暂无执行记录'
  return `${form.value.lastCleanupStatus || '暂无执行记录'}；最近清理记录数：${form.value.lastCleanupRemovedRecords ?? 0}`
})

const formatMiB = (bytes?: number) =>
  `${((bytes ?? 0) / (1024 * 1024)).toFixed(1)} MB`
const formatMBValue = (mb?: number) => `${Number(mb ?? 0).toFixed(1)} MB`
const formatMbps = (mbps?: number) => `${Number(mbps ?? 0).toFixed(1)} Mbps`
const resourceStatusTagColor = (value?: string) => {
  if (value === 'green') return 'green'
  if (value === 'yellow') return 'orange'
  if (value === 'red') return 'red'
  return 'default'
}

const load = async () => {
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    const [settings, usage] = await Promise.all([
      gatewayApi.fetchSystemSettings(),
      gatewayApi.fetchSystemResourceUsage()
    ])
    form.value = settings
    resourceUsage.value = usage
    validateSettings()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载系统设置失败'
  } finally {
    loading.value = false
  }
}

const loadAdminSession = () => {
  if (typeof window === 'undefined') return
  adminSession.value = {
    operator: window.localStorage.getItem('siptunnel.adminOperator') || '',
    token: window.localStorage.getItem('siptunnel.adminToken') || '',
    mfa: window.localStorage.getItem('siptunnel.adminMfa') || ''
  }
}

const saveAdminSession = () => {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(
    'siptunnel.adminOperator',
    adminSession.value.operator || ''
  )
  window.localStorage.setItem(
    'siptunnel.adminToken',
    adminSession.value.token || ''
  )
  window.localStorage.setItem(
    'siptunnel.adminMfa',
    adminSession.value.mfa || ''
  )
  notice.value = '浏览器本地管理会话已保存。'
}

const clearAdminSession = () => {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem('siptunnel.adminOperator')
  window.localStorage.removeItem('siptunnel.adminToken')
  window.localStorage.removeItem('siptunnel.adminMfa')
  adminSession.value = { operator: '', token: '', mfa: '' }
  notice.value = '浏览器本地管理会话已清空。'
}

const save = async () => {
  if (!form.value) return
  if (validateSettings().length > 0) {
    error.value = '请先修正系统设置中的校验问题'
    return
  }
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    form.value = await gatewayApi.updateSystemSettings(form.value)
    resourceUsage.value = await gatewayApi.fetchSystemResourceUsage()
    notice.value = '系统设置已保存并重新回读。'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存系统设置失败'
  } finally {
    saving.value = false
  }
}

watch(form, () => validateSettings(), { deep: true })
onMounted(() => {
  loadAdminSession()
  load()
})
</script>

<style scoped>
.full-card {
  height: 100%;
}
.validation-list {
  margin: 0;
  padding-left: 18px;
}
</style>
