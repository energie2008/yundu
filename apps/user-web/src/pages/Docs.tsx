import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Search,
  BookOpen,
  ChevronRight,
  FileText,
  ChevronDown,
} from 'lucide-react'
import clsx from 'clsx'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface Article {
  id: number
  category: string
  title: string
  body: string
  show: number
  sort: number
  created_at?: string
  updated_at?: string
}

interface Category {
  id: number
  name: string
  sort: number
}

export default function Docs() {
  const [selectedCategory, setSelectedCategory] = useState<string>('')
  const [searchQuery, setSearchQuery] = useState('')
  const [expandedId, setExpandedId] = useState<number | null>(null)

  const { data: catData, isLoading: catLoading } = useQuery<{ categories: Category[] }>({
    queryKey: ['knowledge-categories'],
    queryFn: () => api.get(EP.KNOWLEDGE_CATEGORIES),
    retry: false,
  })

  const { data: artData, isLoading: artLoading } = useQuery<{ items: Article[] }>({
    queryKey: ['knowledge-articles'],
    queryFn: () => api.get(EP.KNOWLEDGE_ARTICLES),
    retry: false,
  })

  const categories = catData?.categories || []
  const articles = artData?.items || []

  const filteredArticles = articles.filter((a) => {
    const matchesSearch =
      searchQuery === '' ||
      a.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
      a.body.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesCategory = selectedCategory === '' || a.category === selectedCategory
    return matchesSearch && matchesCategory
  })

  const getCategoryName = (name: string): string => {
    if (!name) return '未分类'
    return name
  }

  const toggleExpand = (id: number) => {
    setExpandedId(expandedId === id ? null : id)
  }

  return (
    <div className="p-6 max-w-6xl mx-auto">
      {/* Header */}
      <div className="text-center mb-8">
        <div className="inline-flex items-center gap-2 px-4 py-1.5 rounded-full mb-4" style={{ background: 'rgba(124,92,252,0.1)' }}>
          <BookOpen className="w-4 h-4" style={{ color: 'var(--primary)' }} />
          <span className="text-sm" style={{ color: 'var(--primary)' }}>知识库</span>
        </div>
        <h1 className="text-3xl font-bold mb-3" style={{ color: 'var(--foreground)' }}>
          使用文档与帮助中心
        </h1>
        <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
          查找使用教程、客户端配置和常见问题解答
        </p>
      </div>

      {/* Search */}
      <div className="mb-6 relative max-w-2xl mx-auto">
        <Search className="absolute left-4 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
        <input
          type="text"
          placeholder="搜索文章标题或内容..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full h-12 pl-12 pr-4 rounded-lg text-sm outline-none transition-colors"
          style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        {/* Categories sidebar */}
        <div className="lg:col-span-1 h-fit xboard-card">
          <div className="p-4">
            <div className="flex items-center gap-2 mb-3">
              <BookOpen className="w-4 h-4" style={{ color: 'var(--primary)' }} />
              <span className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>分类</span>
            </div>
            <div className="space-y-1">
              <button
                onClick={() => setSelectedCategory('')}
                className={clsx(
                  'w-full flex items-center justify-between px-3 py-2 rounded-lg text-sm transition-colors',
                  selectedCategory === ''
                    ? 'border'
                    : 'hover:bg-opacity-50'
                )}
                style={selectedCategory === ''
                  ? { background: 'rgba(124,92,252,0.08)', color: 'var(--primary)', borderColor: 'rgba(124,92,252,0.3)', borderWidth: '1px', borderStyle: 'solid' }
                  : { color: 'var(--muted-foreground)' }
                }
                onMouseEnter={e => {
                  if (selectedCategory !== '') e.currentTarget.style.background = 'var(--muted)'
                }}
                onMouseLeave={e => {
                  if (selectedCategory !== '') e.currentTarget.style.background = 'transparent'
                }}
              >
                <span className="flex items-center gap-2">
                  <ChevronRight className="w-3 h-3" />
                  <span>全部文章</span>
                </span>
                <span
                  className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'var(--muted)', color: 'var(--muted-foreground)' }}
                >
                  {articles.length}
                </span>
              </button>
              {catLoading ? (
                <div className="space-y-1">
                  <div className="h-9 w-full rounded animate-pulse" style={{ background: 'var(--muted)' }} />
                  <div className="h-9 w-full rounded animate-pulse" style={{ background: 'var(--muted)' }} />
                </div>
              ) : (
                categories.map((c) => {
                  const count = articles.filter((a) => a.category === c.name).length
                  const isSelected = selectedCategory === c.name
                  return (
                    <button
                      key={c.id}
                      onClick={() => setSelectedCategory(c.name)}
                      className={clsx(
                        'w-full flex items-center justify-between px-3 py-2 rounded-lg text-sm transition-colors',
                        isSelected ? 'border' : 'hover:bg-opacity-50'
                      )}
                      style={isSelected
                        ? { background: 'rgba(124,92,252,0.08)', color: 'var(--primary)', borderColor: 'rgba(124,92,252,0.3)', borderWidth: '1px', borderStyle: 'solid' }
                        : { color: 'var(--muted-foreground)' }
                      }
                      onMouseEnter={e => {
                        if (!isSelected) e.currentTarget.style.background = 'var(--muted)'
                      }}
                      onMouseLeave={e => {
                        if (!isSelected) e.currentTarget.style.background = 'transparent'
                      }}
                    >
                      <span className="flex items-center gap-2 truncate">
                        <FileText className="w-3 h-3 shrink-0" />
                        <span className="truncate">{c.name}</span>
                      </span>
                      <span
                        className="text-xs px-2 py-0.5 rounded-full shrink-0"
                        style={{ background: 'var(--muted)', color: 'var(--muted-foreground)' }}
                      >
                        {count}
                      </span>
                    </button>
                  )
                })
              )}
              {!catLoading && categories.length === 0 && (
                <div className="text-center text-xs py-4" style={{ color: 'var(--muted-foreground)' }}>暂无分类</div>
              )}
            </div>
          </div>
        </div>

        {/* Articles list */}
        <div className="lg:col-span-3 space-y-3">
          {artLoading ? (
            <div className="space-y-3">
              <div className="xboard-card p-5">
                <div className="h-5 w-1/3 rounded animate-pulse mb-3" style={{ background: 'var(--muted)' }} />
                <div className="h-4 w-full rounded animate-pulse" style={{ background: 'var(--muted)' }} />
              </div>
              <div className="xboard-card p-5">
                <div className="h-5 w-1/3 rounded animate-pulse mb-3" style={{ background: 'var(--muted)' }} />
                <div className="h-4 w-full rounded animate-pulse" style={{ background: 'var(--muted)' }} />
              </div>
              <div className="xboard-card p-5">
                <div className="h-5 w-1/3 rounded animate-pulse mb-3" style={{ background: 'var(--muted)' }} />
                <div className="h-4 w-full rounded animate-pulse" style={{ background: 'var(--muted)' }} />
              </div>
            </div>
          ) : filteredArticles.length === 0 ? (
            <div className="xboard-card">
              <div className="p-12 text-center">
                <FileText className="w-12 h-12 mx-auto mb-3" style={{ color: 'var(--muted-foreground)' }} />
                <p className="mb-1" style={{ color: 'var(--muted-foreground)' }}>
                  {searchQuery ? '未找到匹配的文章' : '暂无文章'}
                </p>
                <p className="text-xs" style={{ color: 'var(--muted-foreground)' }}>
                  {searchQuery ? '尝试更换搜索关键词' : '请稍后再来查看'}
                </p>
              </div>
            </div>
          ) : (
            filteredArticles.map((article) => {
              const expanded = expandedId === article.id
              return (
                <div
                  key={article.id}
                  className="xboard-card overflow-hidden transition-all"
                >
                  <button
                    onClick={() => toggleExpand(article.id)}
                    className="w-full text-left p-5 transition-colors"
                    onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
                    onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 mb-2 flex-wrap">
                          <span
                            className="text-xs px-2 py-0.5 rounded-full"
                            style={{ background: 'rgba(124,92,252,0.1)', color: 'var(--primary)' }}
                          >
                            {getCategoryName(article.category)}
                          </span>
                        </div>
                        <h3 className="text-base font-semibold mb-1" style={{ color: 'var(--foreground)' }}>
                          {article.title}
                        </h3>
                        <p className="text-sm line-clamp-2" style={{ color: 'var(--muted-foreground)' }}>
                          {article.body}
                        </p>
                      </div>
                      <ChevronDown
                        className={clsx(
                          'w-5 h-5 shrink-0 mt-1 transition-transform',
                          expanded && 'rotate-180'
                        )}
                        style={{ color: 'var(--muted-foreground)' }}
                      />
                    </div>
                  </button>
                  {expanded && (
                    <div className="px-5 pb-5 border-t pt-4" style={{ borderColor: 'var(--border)' }}>
                      <div
                        className="text-sm leading-relaxed whitespace-pre-line"
                        style={{ color: 'var(--foreground)' }}
                      >
                        {article.body}
                      </div>
                    </div>
                  )}
                </div>
              )
            })
          )}
        </div>
      </div>
    </div>
  )
}
