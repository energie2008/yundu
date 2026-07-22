import * as React from 'react'
import { Badge } from '@airport/ui'
import { Check, AlertTriangle, Shield, Zap, Server, Info, ChevronRight } from 'lucide-react'
import type { PresetTemplate } from '@/types/preset'
import { COMPAT_META, BADGE_STYLE } from '@/types/preset'

interface PresetCardProps {
  preset: PresetTemplate
  selected: boolean
  modified?: boolean
  onClick: () => void
}

const PROTOCOL_COLORS: Record<string, string> = {
  vless: 'bg-emerald-500',
  vmess: 'bg-blue-500',
  trojan: 'bg-purple-500',
  ss: 'bg-amber-500',
  shadowsocks: 'bg-amber-500',
  hysteria2: 'bg-rose-500',
  tuic: 'bg-cyan-500',
  anytls: 'bg-violet-500',
  mieru: 'bg-lime-500',
}

export function PresetCard({ preset, selected, modified, onClick }: PresetCardProps) {
  const compat = COMPAT_META[preset.kernel_compat]
  const isDeprecated = preset.deprecated_at && new Date(preset.deprecated_at) < new Date()
  const badgeStyle = preset.badge ? BADGE_STYLE[preset.badge] : null
  const protoColor = PROTOCOL_COLORS[preset.protocol] || 'bg-zinc-500'

  return (
    <button
      type="button"
      onClick={onClick}
      className={`relative w-full text-left p-4 rounded-xl border transition-all duration-200 group ${
        selected
          ? 'border-indigo-500 bg-indigo-950/20 shadow-lg shadow-indigo-900/20'
          : isDeprecated
          ? 'border-red-900/50 bg-red-950/10 hover:border-red-800/50'
          : 'border-zinc-800 bg-zinc-900/50 hover:border-zinc-700 hover:bg-zinc-900'
      }`}
    >
      {selected && (
        <div className="absolute top-3 right-3 w-6 h-6 rounded-full bg-indigo-500 flex items-center justify-center">
          <Check className="w-3.5 h-3.5 text-white" />
        </div>
      )}

      {modified && selected && (
        <div className="absolute top-3 right-11">
          <Badge className="bg-amber-900/60 text-amber-300 border-amber-800/50 text-[10px] px-1.5 py-0.5 h-auto">
            已修改
          </Badge>
        </div>
      )}

      <div className="flex items-start gap-3 mb-2">
        <div className={`w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 ${protoColor}/20 bg-opacity-20 mt-0.5`}>
          <Server className="w-4 h-4" style={{ color: protoColor.replace('bg-', '').includes('emerald') ? '#10b981' : protoColor.includes('blue') ? '#3b82f6' : protoColor.includes('purple') ? '#a855f7' : protoColor.includes('amber') ? '#f59e0b' : protoColor.includes('rose') ? '#f43f5e' : protoColor.includes('cyan') ? '#06b6d4' : protoColor.includes('violet') ? '#8b5cf6' : protoColor.includes('lime') ? '#84cc16' : '#71717a' }} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 flex-wrap mb-1">
            <h4 className="font-semibold text-sm text-zinc-100 truncate">{preset.name}</h4>
            {preset.badge && badgeStyle && (
              <span className={`text-[10px] px-1.5 py-0.5 rounded border ${badgeStyle.bg} ${badgeStyle.color} ${badgeStyle.color.includes('text-') ? '' : ''}`}>
                {preset.badge}
              </span>
            )}
          </div>
          <div className="flex items-center gap-2 text-[11px] text-zinc-500 font-mono">
            <span className="text-zinc-400 uppercase">{preset.protocol}</span>
            <span className="text-zinc-600">+</span>
            <span className="text-zinc-400 uppercase">{preset.transport}</span>
            <span className="text-zinc-600">+</span>
            <span className="text-zinc-400 uppercase">{preset.security}</span>
          </div>
        </div>
      </div>

      <p className="text-xs text-zinc-400 leading-relaxed mb-3 line-clamp-2 min-h-[2rem]">
        {preset.description}
      </p>

      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-1.5">
          <div className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: compat.color }} />
          <span className="text-[10px]" style={{ color: compat.color }}>{compat.label}</span>
        </div>
        <div className="flex items-center gap-1 text-[10px] text-zinc-500 truncate">
          <span className="truncate">{preset.client_support.slice(0, 2).join(' · ')}</span>
          {preset.client_support.length > 2 && (
            <span className="text-zinc-600">+{preset.client_support.length - 2}</span>
          )}
        </div>
      </div>

      {(preset.warnings && preset.warnings.length > 0) && (
        <div className="mt-3 pt-2 border-t border-zinc-800">
          <div className="flex items-start gap-1.5">
            <AlertTriangle className="w-3 h-3 text-amber-500 flex-shrink-0 mt-0.5" />
            <p className="text-[10px] text-amber-400/80 leading-relaxed line-clamp-1">{preset.warnings[0]}</p>
          </div>
        </div>
      )}

      {isDeprecated && (
        <div className="mt-2 p-2 rounded bg-red-950/40 border border-red-900/30">
          <p className="text-[10px] text-red-400 flex items-center gap-1">
            <AlertTriangle className="w-3 h-3" />该预设已标记为弃用，请使用替代方案
          </p>
        </div>
      )}
    </button>
  )
}

interface CompatibilityIndicatorProps {
  spec: {
    protocol: string
    transport: string | { type: string }
    security: string
    kernel_compat?: string
  }
  showDetails?: boolean
}

export function CompatibilityIndicator({ spec, showDetails = false }: CompatibilityIndicatorProps) {
  const transportType = typeof spec.transport === 'string' ? spec.transport : spec.transport?.type || ''
  const compat = detectCompatibility(spec.protocol, transportType, spec.security)
  const meta = COMPAT_META[compat as keyof typeof COMPAT_META] || COMPAT_META.both

  return (
    <div className={`flex items-center gap-2 px-3 py-2 rounded-lg border ${meta.bgColor} ${meta.borderColor}`}>
      <Shield className="w-4 h-4" style={{ color: meta.color }} />
      <div className="min-w-0">
        <div className="text-xs font-medium" style={{ color: meta.color }}>{meta.label}</div>
        {showDetails && <p className="text-[10px] text-zinc-500">{meta.desc}</p>}
      </div>
    </div>
  )
}

function detectCompatibility(proto: string, transport: string, security: string): string {
  if (proto === 'anytls') return 'singbox_only'
  if (proto === 'mieru') return 'experimental'
  if (transport === 'xhttp' && security === 'reality') return 'xray_only'
  if (transport === 'xhttp') return 'experimental'
  if (proto === 'hysteria2' && (security === 'tls' || security === 'none')) return 'both'
  if (proto === 'ss' && transport === 'udp') return 'both'
  return 'both'
}

interface PresetDiffViewerProps {
  modifiedFields: string[]
  presetName?: string
}

export function PresetDiffViewer({ modifiedFields, presetName }: PresetDiffViewerProps) {
  if (modifiedFields.length === 0) return null

  const fieldLabels: Record<string, string> = {
    'protocol': '协议',
    'transport.type': '传输方式',
    'security': '安全层',
    'tls.sni': 'TLS SNI',
    'tls.fingerprint': 'uTLS指纹',
    'tls.cert_mode': 'TLS证书模式',
    'transport.ws.path': 'WS路径',
    'transport.ws.host': 'WS Host',
    'transport.xhttp.mode': 'XHTTP模式',
    'transport.grpc.service_name': 'gRPC服务名',
    'transport.mux.enabled': '多路复用',
    'transport.tcp_brutal.enabled': 'TCP Brutal',
  }

  return (
    <div className="p-3 rounded-lg bg-amber-950/20 border border-amber-900/30 flex items-start gap-2">
      <Info className="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" />
      <div className="text-xs text-amber-300/90">
        {presetName && <span className="font-medium">基于「{presetName}」预设，已修改 {modifiedFields.length} 项：</span>}
        {!presetName && <span>已修改 {modifiedFields.length} 项：</span>}
        <div className="flex flex-wrap gap-1 mt-1">
          {modifiedFields.map(f => (
            <span key={f} className="px-1.5 py-0.5 rounded bg-amber-900/40 text-[10px] text-amber-200 font-mono">
              {fieldLabels[f] || f}
            </span>
          ))}
        </div>
      </div>
    </div>
  )
}
