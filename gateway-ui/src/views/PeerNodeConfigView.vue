<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="对端节点配置">
      <a-alert
        type="info"
        show-icon
        message="全局网络模式决定 transport 承载策略"
        description="映射页只配置本端入口和对端目标；peer 页负责 signaling/media 与模式兼容治理。"
        style="margin-bottom: 12px"
      />
      <a-descriptions bordered :column="2" size="small" style="margin-bottom: 16px">
        <a-descriptions-item label="当前 NetworkMode">{{ networkStatus?.current_network_mode ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="Capability 摘要">{{ capabilitySummaryText }}</a-descriptions-item>
      </a-descriptions>
      <a-space style="margin-bottom: 12px">
        <a-button type="primary" @click="openCreate">新增对端节点</a-button>
        <a-button @click="load">刷新</a-button>
      </a-space>
      <a-table :data-source="peers" row-key="peer_node_id" :pagination="false">
        <a-table-column title="peer_node_id" data-index="peer_node_id" key="peer_node_id" />
        <a-table-column title="peer_name" data-index="peer_name" key="peer_name" />
        <a-table-column title="signaling 地址" key="signaling">
          <template #default="{ record }">{{ record.peer_signaling_ip }}:{{ record.peer_signaling_port }}</template>
        </a-table-column>
        <a-table-column title="media 地址范围" key="media">
          <template #default="{ record }">{{ record.peer_media_ip }}:{{ record.peer_media_port_start }}-{{ record.peer_media_port_end }}</template>
        </a-table-column>
        <a-table-column title="supported_network_mode" data-index="supported_network_mode" key="supported_network_mode" />
        <a-table-column title="enabled" key="enabled">
          <template #default="{ record }"><a-switch :checked="record.enabled" disabled /></template>
        </a-table-column>
        <a-table-column title="操作" key="action">
          <template #default="{ record }">
            <a-space>
              <a-button type="link" @click="openEdit(record)">编辑</a-button>
              <a-popconfirm title="确认删除该 peer？" @confirm="remove(record.peer_node_id)">
                <a-button type="link" danger>删除</a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </a-table-column>
      </a-table>
    </a-card>

    <a-drawer v-model:open="drawerOpen" :title="drawerTitle" width="620" @close="drawerOpen = false">
      <a-form layout="vertical">
        <a-form-item label="peer_node_id"><a-input v-model:value="editing.peer_node_id" :disabled="mode === 'edit'" /></a-form-item>
        <a-form-item label="peer_name"><a-input v-model:value="editing.peer_name" /></a-form-item>
        <a-form-item label="peer_signaling_ip"><a-input v-model:value="editing.peer_signaling_ip" /></a-form-item>
        <a-form-item label="peer_signaling_port"><a-input-number v-model:value="editing.peer_signaling_port" :min="1" :max="65535" style="width: 100%" /></a-form-item>
        <a-form-item label="peer_media_ip"><a-input v-model:value="editing.peer_media_ip" /></a-form-item>
        <a-form-item label="peer_media_port_start"><a-input-number v-model:value="editing.peer_media_port_start" :min="1" :max="65535" style="width: 100%" /></a-form-item>
        <a-form-item label="peer_media_port_end"><a-input-number v-model:value="editing.peer_media_port_end" :min="1" :max="65535" style="width: 100%" /></a-form-item>
        <a-form-item label="supported_network_mode"><a-input v-model:value="editing.supported_network_mode" /></a-form-item>
        <a-form-item label="enabled"><a-switch v-model:checked="editing.enabled" /></a-form-item>
      </a-form>
      <template #footer>
        <a-space style="width: 100%; justify-content: flex-end">
          <a-button @click="drawerOpen = false">取消</a-button>
          <a-button type="primary" @click="save">保存</a-button>
        </a-space>
      </template>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { NodeNetworkStatusPayload, PeerNodeConfig } from '../types/gateway'

const peers = ref<PeerNodeConfig[]>([])
const networkStatus = ref<NodeNetworkStatusPayload>()
const drawerOpen = ref(false)
const mode = ref<'create' | 'edit'>('create')

const emptyPeer = (): PeerNodeConfig => ({
  peer_node_id: '',
  peer_name: '',
  peer_signaling_ip: '',
  peer_signaling_port: 5060,
  peer_media_ip: '',
  peer_media_port_start: 32000,
  peer_media_port_end: 32100,
  supported_network_mode: '',
  enabled: true
})
const editing = reactive<PeerNodeConfig>(emptyPeer())

const drawerTitle = computed(() => (mode.value === 'create' ? '新增对端节点' : '编辑对端节点'))
const capabilitySummaryText = computed(() => {
  if (!networkStatus.value) return '-'
  const summary = networkStatus.value.capability_summary
  return `支持: ${summary.supported.join(', ') || '-'}；不支持: ${summary.unsupported.join(', ') || '-'}`
})

const load = async () => {
  const [peerResult, status] = await Promise.all([gatewayApi.fetchPeers(), gatewayApi.fetchNodeNetworkStatus()])
  peers.value = peerResult.items
  networkStatus.value = status
}

const openCreate = () => {
  mode.value = 'create'
  Object.assign(editing, emptyPeer())
  editing.supported_network_mode = networkStatus.value?.current_network_mode ?? ''
  drawerOpen.value = true
}

const openEdit = (peer: PeerNodeConfig) => {
  mode.value = 'edit'
  Object.assign(editing, JSON.parse(JSON.stringify(peer)))
  drawerOpen.value = true
}

const save = async () => {
  if (mode.value === 'create') {
    await gatewayApi.createPeer(JSON.parse(JSON.stringify(editing)))
    message.success('对端节点已创建')
  } else {
    const { peer_node_id, ...payload } = JSON.parse(JSON.stringify(editing))
    await gatewayApi.updatePeer(peer_node_id, payload)
    message.success('对端节点已更新')
  }
  drawerOpen.value = false
  await load()
}

const remove = async (peerNodeId: string) => {
  await gatewayApi.deletePeer(peerNodeId)
  message.success('对端节点已删除')
  await load()
}

onMounted(load)
</script>
