import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Bell,
  Check,
  CheckCheck,
  ChevronLeft,
  ChevronRight,
  Clock,
  Activity,
  MessageSquare,
  AlertTriangle,
  Loader2,
} from 'lucide-react'
import { Button } from '@airport/ui'
import { useToast } from '@/lib/toast'
import { api } from '@/lib/api'
import {
  EP,
  NotificationItem,
  PaginatedResponse,
  formatTimeAgo,
  formatDateTime,
} from '@/lib/endpoints'
import clsx from 'clsx'

type Filter = 'all' | 'unread'

const filterTabs: { key: Filter; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'unread', label: '未读' },
]

function getNotifIcon(type: string) {
  const t = (type || '').toLowerCase()
  if (t.includes('expir')) return { icon: Clock, color: '#f59e0b' }
  if (t.includes('traffic')) return { icon: Activity, color: 'var(--primary)' }
  if (t.includes('ticket')) return { icon: MessageSquare, color: '#8b5cf6' }
  if (t.includes('order')) return { icon: AlertTriangle, color: 'var(--destructive)' }
  return { icon: Bell, color: 'var(--primary)' }
}

export default function Notifications() {
  const { toast } = useToast()
  const queryClient = useQueryClient()
  const [filter, setFilter] = useState<Filter>('all')
  const [page, setPage] = useState(1)
  const pageSize = 20

  const { data: unreadCount = 0 } = useQuery<number>({
    queryKey: ['notifications-unread-count'],
    queryFn: async () => {
      try {
        const res = await api.get<{ count: number }>(EP.NOTIFICATIONS_UNREAD_COUNT)
        return res.count ?? 0
      } catch {
        return 0
      }
    },
    refetchOnWindowFocus: true,
    retry: false,
  })

  const { data, isLoading } = useQuery<
    PaginatedResponse<NotificationItem> | NotificationItem[]
  >({
    queryKey: ['notifications', page, pageSize, filter],
    queryFn: async () => {
      const params: Record<string, string | number | boolean | undefined> = {
        page,
        page_size: pageSize,
      }
      if (filter === 'unread') params.filter = 'unread'
      const res = await api.get<
        PaginatedResponse<NotificationItem> | NotificationItem[]
      >(EP.NOTIFICATIONS, { params })
      return res
    },
  })

  const items: NotificationItem[] = Array.isArray(data)
    ? data
    : data?.items || []
  const total: number = Array.isArray(data) ? data.length : data?.total || 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const markReadMutation = useMutation({
    mutationFn: (id: string) => api.post(EP.NOTIFICATION_READ(id)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications-unread-count'] })
      queryClient.invalidateQueries({ queryKey: ['notifications'] })
    },
    onError: (e: any) => {
      toast({ title: '操作失败', description: e.message, variant: 'destructive' })
    },
  })

  const markAllReadMutation = useMutation({
    mutationFn: () => api.post(EP.NOTIFICATIONS_READ_ALL),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications-unread-count'] })
      queryClient.invalidateQueries({ queryKey: ['notifications'] })
      toast({ title: '已全部标记为已读', variant: 'success' })
    },
    onError: (e: any) => {
      toast({ title: '操作失败', description: e.message, variant: 'destructive' })
    },
  })

  const handleFilterChange = (f: Filter) => {
    setFilter(f)
    setPage(1)
  }

  return (
    <div className="p-6 max-w-4xl mx-auto space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold flex items-center gap-3" style={{ color: 'var(--foreground)' }}>
            <div
              className="w-10 h-10 rounded-xl flex items-center justify-center shadow-sm"
              style={{ background: 'var(--primary)' }}
            >
              <Bell className="w-5 h-5 text-white" />
            </div>
            通知中心
            {unreadCount > 0 && (
              <span
                className="text-xs font-bold text-white rounded-full px-2 py-0.5"
                style={{ background: 'var(--destructive)' }}
              >
                {unreadCount > 99 ? '99+' : unreadCount} 未读
              </span>
            )}
          </h1>
          <p className="mt-1 text-sm" style={{ color: 'var(--muted-foreground)' }}>
            查看您的所有通知消息
          </p>
        </div>
        {unreadCount > 0 && (
          <Button
            className="h-10 px-4 rounded-lg border-0 text-sm font-medium flex items-center gap-2 shadow-sm"
            style={{
              backgroundColor: 'rgba(124,92,252,0.1)',
              color: 'var(--primary)',
            }}
            onClick={() => markAllReadMutation.mutate()}
            disabled={markAllReadMutation.isPending}
            onMouseEnter={e => (e.currentTarget.style.backgroundColor = 'rgba(124,92,252,0.15)')}
            onMouseLeave={e => (e.currentTarget.style.backgroundColor = 'rgba(124,92,252,0.1)')}
          >
            {markAllReadMutation.isPending ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <CheckCheck className="w-4 h-4" />
            )}
            全部已读
          </Button>
        )}
      </div>

      {/* Filter tabs */}
      <div className="flex items-center gap-1 border-b" style={{ borderColor: 'var(--border)' }}>
        {filterTabs.map((tab) => (
          <button
            key={tab.key}
            onClick={() => handleFilterChange(tab.key)}
            className={clsx(
              'px-4 py-2.5 text-sm font-medium transition-colors relative',
              filter === tab.key ? '' : 'hover:opacity-80'
            )}
            style={{
              color: filter === tab.key ? 'var(--primary)' : 'var(--muted-foreground)',
              borderBottom: filter === tab.key ? '2px solid var(--primary)' : '2px solid transparent',
            }}
          >
            {tab.label}
            {tab.key === 'unread' && unreadCount > 0 && (
              <span
                className="ml-1.5 text-[10px] font-bold text-white rounded-full px-1.5 py-0.5"
                style={{ background: 'var(--destructive)' }}
              >
                {unreadCount > 99 ? '99+' : unreadCount}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* List */}
      {isLoading ? (
        <div className="space-y-3">
          {[0, 1, 2, 3].map((i) => (
            <div
              key={i}
              className="p-4 rounded-xl animate-pulse"
              style={{ backgroundColor: 'var(--muted)', border: '1px solid var(--border)' }}
            >
              <div className="flex gap-3">
                <div className="w-10 h-10 rounded-lg flex-shrink-0" style={{ backgroundColor: 'var(--border)' }} />
                <div className="flex-1 space-y-2">
                  <div className="h-4 w-1/3 rounded" style={{ backgroundColor: 'var(--border)' }} />
                  <div className="h-3 w-2/3 rounded" style={{ backgroundColor: 'var(--border)' }} />
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : items.length === 0 ? (
        <div
          className="xboard-card py-16 text-center"
        >
          <div
            className="w-16 h-16 rounded-2xl flex items-center justify-center mx-auto mb-4"
            style={{ backgroundColor: 'var(--muted)' }}
          >
            <Bell className="w-8 h-8" style={{ color: 'var(--muted-foreground)' }} />
          </div>
          <h3 className="text-lg font-semibold mb-2" style={{ color: 'var(--foreground)' }}>
            {filter === 'unread' ? '没有未读通知' : '暂无通知'}
          </h3>
          <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
            {filter === 'unread' ? '所有通知都已读过了' : '新的通知会出现在这里'}
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {items.map((notif) => {
            const { icon: Icon, color: iconColor } = getNotifIcon(notif.type)
            return (
              <div
                key={notif.id}
                className="p-4 rounded-xl transition-all duration-200 group"
                style={{
                  backgroundColor: notif.is_read ? 'var(--card)' : 'rgba(124,92,252,0.05)',
                  border: `1px solid ${notif.is_read ? 'var(--border)' : 'rgba(124,92,252,0.2)'}`,
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.backgroundColor = notif.is_read ? 'var(--muted)' : 'rgba(124,92,252,0.1)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.backgroundColor = notif.is_read ? 'var(--card)' : 'rgba(124,92,252,0.05)'
                }}
              >
                <div className="flex items-start gap-3">
                  <div
                    className="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0"
                    style={{ backgroundColor: 'var(--muted)' }}
                  >
                    <Icon className="w-5 h-5" style={{ color: iconColor }} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-start justify-between gap-2">
                      <p
                        className="text-sm font-medium"
                        style={{ color: notif.is_read ? 'var(--muted-foreground)' : 'var(--foreground)' }}
                      >
                        {notif.title}
                      </p>
                      {!notif.is_read && (
                        <button
                          onClick={() => markReadMutation.mutate(notif.id)}
                          disabled={markReadMutation.isPending}
                          className="text-xs flex-shrink-0 flex items-center gap-1 px-2 py-1 rounded-lg transition-colors"
                          style={{ color: 'var(--primary)' }}
                          onMouseEnter={(e) => {
                            e.currentTarget.style.backgroundColor = 'rgba(124,92,252,0.1)'
                          }}
                          onMouseLeave={(e) => {
                            e.currentTarget.style.backgroundColor = 'transparent'
                          }}
                        >
                          <Check className="w-3 h-3" />
                          标记已读
                        </button>
                      )}
                    </div>
                    <p
                      className="text-sm mt-1 whitespace-pre-line leading-relaxed"
                      style={{ color: 'var(--muted-foreground)' }}
                    >
                      {notif.content}
                    </p>
                    <div className="flex items-center gap-2 mt-2">
                      <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>
                        {formatTimeAgo(notif.created_at)}
                      </span>
                      <span style={{ color: 'var(--muted-foreground)' }}>·</span>
                      <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>
                        {formatDateTime(notif.created_at)}
                      </span>
                    </div>
                  </div>
                  {!notif.is_read && (
                    <div
                      className="w-2 h-2 rounded-full flex-shrink-0 mt-1.5"
                      style={{ backgroundColor: 'var(--primary)' }}
                    />
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between pt-2">
          <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
            共 {total} 条，第 {page}/{totalPages} 页
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
              className="p-2 rounded-lg transition-colors disabled:opacity-40"
              style={{ color: 'var(--muted-foreground)', backgroundColor: 'var(--card)', border: '1px solid var(--border)' }}
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <button
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
              className="p-2 rounded-lg transition-colors disabled:opacity-40"
              style={{ color: 'var(--muted-foreground)', backgroundColor: 'var(--card)', border: '1px solid var(--border)' }}
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
