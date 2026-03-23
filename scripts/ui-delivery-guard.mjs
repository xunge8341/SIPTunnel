#!/usr/bin/env node
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const repoRoot = path.resolve(path.dirname(__filename), '..')
const args = new Map()
for (let i = 2; i < process.argv.length; i += 1) {
  const key = process.argv[i]
  if (!key.startsWith('--')) continue
  const next = process.argv[i + 1]
  if (!next || next.startsWith('--')) {
    args.set(key, 'true')
  } else {
    args.set(key, next)
    i += 1
  }
}

const mode = (args.get('--mode') || 'verify').toLowerCase()
const jsonStdout = args.has('--json-stdout')
const reportPathArg = args.get('--report') || ''
const allowMissingEmbeddedMetadata = args.has('--allow-missing-embedded-metadata')
const timestamp = new Date().toISOString().replace(/[:.]/g, '-').replace('T', '_').replace('Z', '')
const reportPath = reportPathArg
  ? path.resolve(repoRoot, reportPathArg)
  : path.join(repoRoot, 'artifacts', 'acceptance', `ui-delivery-guard-${timestamp}.json`)

const legacyFiles = [
  'gateway-ui/src/views/ConfigGovernanceView.vue',
  'gateway-ui/src/views/ConfigTransferView.vue',
  'gateway-ui/src/views/NetworkConfigView.vue',
  'gateway-ui/src/views/NodeStatusView.vue',
  'gateway-ui/src/views/__tests__/NodeStatusView.spec.ts',
  'gateway-ui/src/api/__tests__/gatewayConfigTransfer.spec.ts',
]

const forbiddenSymbols = [
  'exportConfigJson',
  'importConfigJson',
  'downloadConfigTemplate',
  'fetchConfigGovernance',
  'rollbackConfig',
  'exportConfigYaml',
  'fetchNetworkConfig',
  'updateNetworkConfig',
  'createDiagnosticExport',
  'fetchDiagnosticExport',
  'retryDiagnosticExport',
]

const allowedLegacyOnlyFiles = new Set([
  'gateway-ui/src/api/mockGateway.ts',
  'gateway-ui/src/api/__tests__/mockGateway.spec.ts',
  'gateway-ui/src/types/gateway.ts',
  'gateway-ui/src/utils/networkConfig.ts',
  'gateway-ui/src/utils/__tests__/networkConfig.spec.ts',
])

const readText = (filePath) => {
  try {
    return fs.readFileSync(filePath, 'utf8')
  } catch {
    return ''
  }
}

const ensureDir = (filePath) => {
  fs.mkdirSync(path.dirname(filePath), { recursive: true })
}

const scanFiles = (dir, files = []) => {
  if (!fs.existsSync(dir)) return files
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      scanFiles(full, files)
      continue
    }
    if (!/\.(ts|tsx|vue|js|mjs|cjs)$/.test(entry.name)) continue
    files.push(full)
  }
  return files
}

const removedLegacyFiles = []
if (mode === 'repair') {
  for (const rel of legacyFiles) {
    const abs = path.join(repoRoot, rel)
    if (fs.existsSync(abs)) {
      fs.rmSync(abs, { force: true })
      removedLegacyFiles.push(rel)
    }
  }
}

const remainingLegacyFiles = legacyFiles.filter((rel) => fs.existsSync(path.join(repoRoot, rel)))
const symbolHits = []
for (const full of scanFiles(path.join(repoRoot, 'gateway-ui', 'src'))) {
  const rel = path.relative(repoRoot, full).replace(/\\/g, '/')
  if (allowedLegacyOnlyFiles.has(rel)) continue
  const text = readText(full)
  for (const symbol of forbiddenSymbols) {
    if (text.includes(symbol)) {
      symbolHits.push({ file: rel, symbol })
    }
  }
}

const uiBaseText = readText(path.join(repoRoot, 'gateway-ui', 'src', 'utils', 'uiBase.ts'))
const viteText = readText(path.join(repoRoot, 'gateway-ui', 'vite.config.ts'))
const distIndexPath = path.join(repoRoot, 'gateway-ui', 'dist', 'index.html')
const embeddedIndexPath = path.join(repoRoot, 'gateway-server', 'internal', 'server', 'embedded-ui', 'index.html')
const embeddedMetaPath = path.join(repoRoot, 'gateway-server', 'internal', 'server', 'embedded-ui', '.siptunnel-ui-embed.json')
const distIndex = readText(distIndexPath)
const embeddedIndex = readText(embeddedIndexPath)

const checks = {
  embedded_metadata_actual_present: fs.existsSync(embeddedMetaPath),
  legacy_files_absent: remainingLegacyFiles.length === 0,
  active_ui_has_no_legacy_api_refs: symbolHits.length === 0,
  router_base_meta_source_present: uiBaseText.includes('meta[name="siptunnel-ui-base-path"]'),
  vite_relative_base_present: viteText.includes("base: './'"),
  dist_relative_assets_present: distIndex ? distIndex.includes('./assets/') : true,
  embedded_relative_assets_present: embeddedIndex ? embeddedIndex.includes('./assets/') : true,
  embedded_base_meta_present: embeddedIndex ? embeddedIndex.includes('meta name="siptunnel-ui-base-path"') : true,
  embedded_metadata_present: fs.existsSync(embeddedMetaPath) || allowMissingEmbeddedMetadata,
}

const enforcedChecks = Object.entries(checks).filter(([name]) => name !== 'embedded_metadata_actual_present')
const failing = enforcedChecks.filter(([, passed]) => !passed).map(([name]) => name)
const status = failing.length === 0 ? 'pass' : 'fail'
const detailParts = []
if (removedLegacyFiles.length > 0) detailParts.push(`removed=${removedLegacyFiles.length}`)
if (remainingLegacyFiles.length > 0) detailParts.push(`legacy_files=${remainingLegacyFiles.length}`)
if (symbolHits.length > 0) detailParts.push(`active_legacy_refs=${symbolHits.length}`)
if (!detailParts.length) detailParts.push('no legacy drift detected')

const report = {
  generated_at: new Date().toISOString(),
  mode,
  status,
  detail: detailParts.join('; '),
  removed_legacy_files: removedLegacyFiles,
  remaining_legacy_files: remainingLegacyFiles,
  active_legacy_symbol_hits: symbolHits,
  checks,
}

ensureDir(reportPath)
fs.writeFileSync(reportPath, JSON.stringify(report, null, 2), 'utf8')

if (jsonStdout) {
  process.stdout.write(JSON.stringify({ ...report, report_path: path.relative(repoRoot, reportPath).replace(/\\/g, '/') }))
} else {
  console.log(`[ui-delivery-guard] mode=${mode} status=${status} detail=${report.detail}`)
  console.log(`[ui-delivery-guard] report=${path.relative(repoRoot, reportPath).replace(/\\/g, '/')}`)
  if (removedLegacyFiles.length > 0) {
    console.log(`[ui-delivery-guard] removed legacy files: ${removedLegacyFiles.join(', ')}`)
  }
}

if (status !== 'pass') {
  process.exit(1)
}
