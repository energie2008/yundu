import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Megaphone,
  Pin,
  ChevronRight,
  Loader2,
} from 'lucide-react'
import {
  Badge,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@airport/ui'
import { api } from '@/lib/api'
import {
  EP,
  AnnouncementItem,
  PaginatedResponse,
  formatDateTime,
  formatTimeAgo,
} from '@/lib/endpoints'

function sortByPinned(items: AnnouncementItem[]): AnnouncementItem[] {
  return [...items].sort((a, b) => {
    if (!!b.is_pinned !== !!a.is_pinned) return (b.is_pinned ? 1 : 0) - (a.is_pinned ? 1 : 0)
    return new Date(b.published_at || b.created_at || '').getTime() - new Date(a.published_at || a.created_at || '').getTime()
  })
}

export default function Announcements() {
  const queryClient = useQueryClient()
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const { data, isLoading } = useQuery<
    PaginatedResponse<AnnouncementItem> | AnnouncementItem[]
  >({
    queryKey: ['announcements'],
    queryFn: async () => {
      const res = await api.get<
        PaginatedResponse<AnnouncementItem> | AnnouncementItem[]
      >(EP.ANNOUNCEMENTS)
      return res
    },
  })

  const items: AnnouncementItem[] = Array.isArray(data)
    ? sortByPinned(data)
    : sortByPinned(data?.items || [])

  const detailQuery = useQuery<AnnouncementItem>({
    queryKey: ['announcement-detail', selectedId],
    queryFn: () => api.get<AnnouncementItem>(EP.ANNOUNCEMENT_DETAIL(selectedId!)),
    enabled: !!selectedId,
  })

  const markReadMutation = useMutation({
    mutationFn: (id: string) => api.post(EP.ANNOUNCEMENT_READ(id)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['announcements'] })
    },
  })

  const handleOpen = (item: AnnouncementItem) => {
    setSelectedId(item.id)
    if (!item.is_read) {
      markReadMutation.mutate(item.id)
    }
  }

  const handleClose = () => {
    setSelectedId(null)
  }

  const detail = detailQuery.data

  return (
    <div className="p-6 max-w-4xl mx-auto space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-xl font-bold flex items-center gap-3" style={{ color: 'var(--foreground)' }}>
          <div
            className="w-10 h-10 rounded-xl flex items-center justify-center shadow-sm"
            style={{ background: 'var(--primary)' }}
          >
            <Megaphone className="w-5 h-5 text-white" />
          </div>
          系统公告
        </h1>
        <p className="mt-1 text-sm" style={{ color: 'var(--muted-foreground)' }}>
          了解最新的平台动态与通知
        </p>
      </div>

      {/* List */}
      <div className="xboard-card">
        <div className="p-6">
          {isLoading ? (
            <div className="space-y-3">
              {[0, 1, 2].map((i) => (
                <div
                  key={i}
                  className="p-4 rounded-xl animate-pulse"
                  style={{ backgroundColor: 'var(--muted)' }}
                >
                  <div className="h-4 w-1/3 rounded mb-2" style={{ backgroundColor: 'var(--border)' }} />
                  <div className="h-3 w-2/3 rounded" style={{ backgroundColor: 'var(--border)' }} />
                </div>
              ))}
            </div>
          ) : items.length === 0 ? (
            <div className="text-center py-12">
              <div
                className="w-16 h-16 rounded-2xl flex items-center justify-center mx-auto mb-4"
                style={{ backgroundColor: 'var(--muted)' }}
              >
                <Megaphone className="w-8 h-8" style={{ color: 'var(--muted-foreground)' }} />
              </div>
              <h3 className="text-lg font-semibold mb-2" style={{ color: 'var(--foreground)' }}>
                暂无公告
              </h3>
              <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
                目前没有新的公告信息
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {items.map((item) => (
                <button
                  key={item.id}
                  className="w-full p-4 rounded-xl text-left transition-all duration-200 group"
                  style={{
                    backgroundColor: item.is_read ? 'var(--muted)' : 'rgba(124,92,252,0.05)',
                    border: `1px solid ${item.is_read ? 'transparent' : 'rgba(124,92,252,0.2)'}`,
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.backgroundColor = item.is_read ? 'var(--border)' : 'rgba(124,92,252,0.1)'
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.backgroundColor = item.is_read ? 'var(--muted)' : 'rgba(124,92,252,0.05)'
                  }}
                  onClick={() => handleOpen(item)}
                >
                  <div className="flex items-start gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1.5 flex-wrap">
                        {item.is_pinned && (
                          <Badge className="border-0 text-xs flex items-center gap-1" style={{ backgroundColor: 'rgba(245,158,11,0.1)', color: '#f59e0b' }}>
                            <Pin className="w-3 h-3" />
                            置顶
                          </Badge>
                        )}
                        {!item.is_read && (
                          <Badge className="border-0 text-xs" style={{ backgroundColor: 'rgba(124,92,252,0.1)', color: 'var(--primary)' }}>
                            新
                          </Badge>
                        )}
                        <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>
                          {formatTimeAgo(item.published_at || item.created_at || '')}
                        </span>
                      </div>
                      <h3
                        className="font-medium line-clamp-1 transition-colors"
                        style={{ color: item.is_read ? 'var(--muted-foreground)' : 'var(--foreground)' }}
                      >
                        {item.title}
                      </h3>
                      <p className="text-sm mt-1 line-clamp-2" style={{ color: 'var(--muted-foreground)' }}>
                        {item.summary || item.content || '点击查看详情'}
                      </p>
                    </div>
                    <ChevronRight
                      className="w-5 h-5 flex-shrink-0 transition-all group-hover:translate-x-1"
                      style={{ color: 'var(--muted-foreground)' }}
                    />
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Detail Dialog */}
      <Dialog open={!!selectedId} onOpenChange={(open) => !open && handleClose()}>
        <DialogContent className="xboard-card" style={{ background: 'var(--card)', borderColor: 'var(--border)', maxWidth: '640px' }}>
          <DialogHeader>
            <DialogTitle style={{ color: 'var(--foreground)' }} className="flex items-center gap-2 pr-6">
              {detail?.is_pinned && (
                <Badge className="border-0 text-xs flex items-center gap-1" style={{ backgroundColor: 'rgba(245,158,11,0.1)', color: '#f59e0b' }}>
                  <Pin className="w-3 h-3" />
                  置顶
                </Badge>
              )}
              {detailQuery.isLoading ? '加载中...' : detail?.title || '公告详情'}
            </DialogTitle>
          </DialogHeader>
          <div className="mt-2">
            {detailQuery.isLoading ? (
              <div className="py-8 text-center">
                <Loader2 className="w-6 h-6 mx-auto animate-spin" style={{ color: 'var(--muted-foreground)' }} />
              </div>
            ) : detail ? (
              <>
                <div className="flex items-center gap-3 mb-4 text-xs" style={{ color: 'var(--muted-foreground)' }}>
                  <span>发布于 {formatDateTime(detail.published_at || detail.created_at || '')}</span>
                  {detail.author && <span>· {detail.author}</span>}
                </div>
                <div
                  className="text-sm leading-relaxed whitespace-pre-line"
                  style={{ color: 'var(--foreground)' }}
                >
                  {detail.content || detail.summary || '暂无内容'}
                </div>
              </>
            ) : null}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
