import { useCallback, useState, useEffect } from 'react'
import { Save, Trash2, RefreshCw } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, Button, Input, Textarea, Skeleton } from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { ADMIN_CARD, ADMIN_BORDER, ADMIN_TEXT, ADMIN_TEXT_SECONDARY, ADMIN_SUCCESS, ADMIN_DANGER } from '@/lib/theme'

interface AgentUpgradeManifest {
  version: string
  download_url: string
  sha256?: string
  release_note?: string
  force_update?: boolean
}

const emptyManifest: AgentUpgradeManifest = { version: '', download_url: '', sha256: '', release_note: '', force_update: false }

export default function AgentUpgrade() {
  const [form, setForm] = useState<AgentUpgradeManifest>(emptyManifest)
  const [hasManifest, setHasManifest] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.get<AgentUpgradeManifest>(EP.AGENT_UPGRADE_MANIFEST)
      if (data && data.version) {
        setForm(data)
        setHasManifest(true)
      } else {
        setForm(emptyManifest)
        setHasManifest(false)
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const save = async () => {
    setSaving(true)
    setMsg('')
    try {
      await api.put(EP.AGENT_UPGRADE_MANIFEST, form)
      setMsg('版本规格已更新，节点将在下一次心跳/5 分钟内自动升级')
      setHasManifest(true)
    } catch (e) {
      setMsg(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    setSaving(true)
    setMsg('')
    try {
      await api.delete(EP.AGENT_UPGRADE_MANIFEST)
      setForm(emptyManifest)
      setHasManifest(false)
      setMsg('已停用自动升级')
    } catch (e) {
      setMsg(e instanceof Error ? e.message : '删除失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Agent 升级版本库</h2>
          <p className={`text-sm ${ADMIN_TEXT_SECONDARY}`}>
            {hasManifest ? '当前已启用自动升级' : '未配置升级版本，节点保持当前版本'}
          </p>
        </div>
        <Button onClick={load} disabled={loading} variant="outline">
          <RefreshCw className="w-4 h-4 mr-1" />
          重新加载
        </Button>
      </div>

      {msg && <div className={`text-sm ${msg.includes('失败') || msg.includes('Error') ? ADMIN_DANGER : ADMIN_SUCCESS}`}>{msg}</div>}

      {loading ? (
        <Skeleton className="h-48" />
      ) : (
        <Card className={ADMIN_CARD}>
          <CardHeader>
            <CardTitle>版本规格（info.json）</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div>
              <label className={`text-sm ${ADMIN_TEXT_SECONDARY}`}>版本号（如 0.7.17）</label>
              <Input
                value={form.version}
                onChange={(e) => setForm({ ...form, version: e.target.value })}
                placeholder="0.7.17"
              />
            </div>
            <div>
              <label className={`text-sm ${ADMIN_TEXT_SECONDARY}`}>下载地址（可直接指向 GitHub Release）</label>
              <Input
                value={form.download_url}
                onChange={(e) => setForm({ ...form, download_url: e.target.value })}
                placeholder="https://github.com/energie2008/yundu/releases/download/v0.7.17/node-agent-amd64"
              />
            </div>
            <div>
              <label className={`text-sm ${ADMIN_TEXT_SECONDARY}`}>SHA256（可选，强烈建议）</label>
              <Input
                value={form.sha256 ?? ''}
                onChange={(e) => setForm({ ...form, sha256: e.target.value })}
                placeholder="64 位十六进制"
              />
            </div>
            <div>
              <label className={`text-sm ${ADMIN_TEXT_SECONDARY}`}>发布说明</label>
              <Textarea
                value={form.release_note ?? ''}
                onChange={(e) => setForm({ ...form, release_note: e.target.value })}
                rows={3}
              />
            </div>
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={!!form.force_update}
                onChange={(e) => setForm({ ...form, force_update: e.target.checked })}
              />
              <span className={`text-sm ${ADMIN_TEXT}`}>强制更新（版本相同也升级）</span>
            </div>
            <div className="flex gap-2 pt-2">
              <Button onClick={save} disabled={saving || !form.version || !form.download_url}>
                <Save className="w-4 h-4 mr-1" />
                保存并发布
              </Button>
              {hasManifest && (
                <Button onClick={remove} disabled={saving} variant="destructive">
                  <Trash2 className="w-4 h-4 mr-1" />
                  停用
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
