import { useState, useEffect, useCallback, useRef } from 'react'
import { listFeed, listSources, reactToPost, type FeedItem, type Source } from '../api'

// ---------- read-state (localStorage) ----------

const LS_KEY = 'feedium_read'
const MAX_STORED = 5000

function loadReadIds(): Set<string> {
  try {
    const raw = localStorage.getItem(LS_KEY)
    return raw ? new Set(JSON.parse(raw) as string[]) : new Set()
  } catch {
    return new Set()
  }
}

function saveReadIds(ids: Set<string>) {
  try {
    // keep only the most-recent MAX_STORED to avoid unbounded growth
    const arr = Array.from(ids).slice(-MAX_STORED)
    localStorage.setItem(LS_KEY, JSON.stringify(arr))
  } catch { /* quota exceeded – ignore */ }
}

function useReadPosts() {
  const [readIds, setReadIds] = useState<Set<string>>(() => loadReadIds())

  const markRead = useCallback((id: string) => {
    setReadIds(prev => {
      if (prev.has(id)) return prev
      const next = new Set(prev)
      next.add(id)
      saveReadIds(next)
      return next
    })
  }, [])

  const markAllRead = useCallback((ids: string[]) => {
    setReadIds(prev => {
      const next = new Set(prev)
      ids.forEach(id => next.add(id))
      saveReadIds(next)
      return next
    })
  }, [])

  return { readIds, markRead, markAllRead }
}

// ---------- helpers ----------

type Period = 'today' | 'yesterday' | '7d' | 'all'

const PERIODS: { key: Period; label: string }[] = [
  { key: 'today',     label: 'Сегодня' },
  { key: 'yesterday', label: 'Вчера' },
  { key: '7d',        label: '7 дней' },
  { key: 'all',       label: 'Всё' },
]

function periodToRange(period: Period): { from?: string; to?: string } {
  const now = new Date()
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  if (period === 'today') {
    return { from: startOfToday.toISOString() }
  }
  if (period === 'yesterday') {
    const start = new Date(startOfToday)
    start.setDate(start.getDate() - 1)
    return { from: start.toISOString(), to: startOfToday.toISOString() }
  }
  if (period === '7d') {
    const start = new Date(startOfToday)
    start.setDate(start.getDate() - 7)
    return { from: start.toISOString() }
  }
  return {}
}

function sourceLabel(sources: Source[], sourceId: string): string {
  const s = sources.find(s => s.id === sourceId)
  if (!s) return sourceId.slice(0, 8) + '…'
  const cfg = s.config as Record<string, unknown>
  if (cfg.telegramChannel) return `@${(cfg.telegramChannel as { username?: string }).username ?? sourceId}`
  if (cfg.telegramGroup)   return `@${(cfg.telegramGroup as { username?: string }).username ?? sourceId}`
  if (cfg.rss)             return (cfg.rss as { feedUrl?: string }).feedUrl?.replace(/^https?:\/\//, '').slice(0, 30) ?? sourceId
  if (cfg.html)            return (cfg.html as { url?: string }).url?.replace(/^https?:\/\//, '').slice(0, 30) ?? sourceId
  return sourceId.slice(0, 8) + '…'
}

function scoreColor(score: number): string {
  if (score >= 0.7) return 'bg-green-100 text-green-800'
  if (score >= 0.4) return 'bg-yellow-100 text-yellow-800'
  return 'bg-red-100 text-red-800'
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

// ---------- FeedCard ----------

interface FeedCardProps {
  item: FeedItem
  sources: Source[]
  isRead: boolean
  onRead: (id: string) => void
  onReact: (id: string, signal: 'like' | 'dislike') => void
}

function FeedCard({ item, sources, isRead, onRead, onReact }: FeedCardProps) {
  const [expanded, setExpanded] = useState(false)
  const preview = item.text.slice(0, 280)
  const hasMore = item.text.length > 280

  const handleToggle = () => {
    setExpanded(e => !e)
    if (!isRead) onRead(item.postId)
  }

  const handleReact = (signal: 'like' | 'dislike') => {
    onReact(item.postId, signal)
    if (!isRead) onRead(item.postId)
  }

  return (
    <div
      className={`rounded-lg border p-4 shadow-sm transition-all ${
        isRead
          ? 'bg-gray-50 border-gray-100 opacity-60'
          : 'bg-white border-gray-200 hover:shadow-md'
      }`}
    >
      <div className="flex items-start justify-between gap-3 mb-2">
        <div className="flex items-center gap-2 text-sm text-gray-500 min-w-0">
          {item.author && <span className="font-medium text-gray-700 shrink-0">{item.author}</span>}
          <span className="shrink-0">{formatDate(item.publishedAt)}</span>
          <span className="text-xs text-gray-400 truncate">{sourceLabel(sources, item.sourceId)}</span>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <button
            onClick={() => handleReact('like')}
            title="Нравится"
            className={`text-sm transition-colors ${
              item.reaction === 'like'
                ? 'text-green-600'
                : 'text-gray-300 hover:text-green-500'
            }`}
          >
            +
          </button>
          <button
            onClick={() => handleReact('dislike')}
            title="Не интересно — убрать из ленты"
            className={`text-sm transition-colors ${
              item.reaction === 'dislike'
                ? 'text-red-500'
                : 'text-gray-300 hover:text-red-400'
            }`}
          >
            −
          </button>
          {item.url && (
            <a
              href={item.url}
              target="_blank"
              rel="noopener noreferrer"
              onClick={() => { if (!isRead) onRead(item.postId) }}
              className="text-xs text-gray-400 hover:text-blue-600 transition-colors"
              title="Открыть оригинал"
            >
              ↗
            </a>
          )}
          <span className={`text-xs font-semibold px-2 py-0.5 rounded-full ${scoreColor(item.score)}`}>
            {item.score.toFixed(2)}
          </span>
        </div>
      </div>

      <button
        onClick={handleToggle}
        className="w-full text-left"
      >
        <p className="text-gray-800 text-sm leading-relaxed whitespace-pre-wrap">
          {expanded ? item.text : preview}
          {!expanded && hasMore && '…'}
        </p>
      </button>

      {hasMore && (
        <button
          onClick={handleToggle}
          className="mt-1 text-xs text-blue-600 hover:underline"
        >
          {expanded ? 'Свернуть' : 'Читать'}
        </button>
      )}
    </div>
  )
}

// ---------- FeedView ----------

export default function FeedView() {
  const [items, setItems] = useState<FeedItem[]>([])
  const [sources, setSources] = useState<Source[]>([])
  const [nextToken, setNextToken] = useState<string | undefined>()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [minScore, setMinScore] = useState(0)
  const [period, setPeriod] = useState<Period>('today')
  const [showRead, setShowRead] = useState(false)

  const { readIds, markRead, markAllRead } = useReadPosts()

  const handleReact = useCallback((postId: string, signal: 'like' | 'dislike') => {
    // Optimistic update
    setItems(prev => prev.map(item =>
      item.postId === postId
        ? { ...item, reaction: item.reaction === signal ? '' : signal }
        : item
    ))
    // Disliked posts disappear from unread view — no need to refresh
    reactToPost(postId, signal).catch(() => {
      // Rollback on failure
      setItems(prev => prev.map(item =>
        item.postId === postId ? { ...item, reaction: '' } : item
      ))
    })
  }, [])

  // keep a ref so markAllRead inside load() closure is stable
  const markAllReadRef = useRef(markAllRead)
  markAllReadRef.current = markAllRead

  useEffect(() => {
    listSources().then(r => setSources(r.items ?? [])).catch(() => {})
  }, [])

  const load = useCallback(async (token?: string) => {
    setLoading(true)
    setError(null)
    try {
      const range = periodToRange(period)
      const res = await listFeed({
        minScore: minScore || undefined,
        from: range.from,
        to: range.to,
        pageSize: 50,
        pageToken: token,
      })
      setItems(prev => token ? [...prev, ...(res.items ?? [])] : (res.items ?? []))
      setNextToken(res.nextPageToken || undefined)
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }, [minScore, period])

  useEffect(() => { load() }, [load])

  const unread = items.filter(i => !readIds.has(i.postId) && i.reaction !== 'dislike')
  const visible = showRead
    ? items.filter(i => i.reaction !== 'dislike')
    : unread

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h1 className="text-2xl font-bold text-gray-900">Feed</h1>
        <div className="flex items-center gap-2">
          {unread.length > 0 && (
            <button
              onClick={() => markAllRead(items.map(i => i.postId))}
              className="px-3 py-1 text-gray-500 hover:text-gray-700 rounded text-sm hover:bg-gray-100"
              title="Отметить все прочитанными"
            >
              ✓ Все прочитаны
            </button>
          )}
          <button
            onClick={() => load()}
            className="px-3 py-1 bg-blue-600 text-white rounded text-sm hover:bg-blue-700"
          >
            Refresh
          </button>
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-3 mb-4 p-3 bg-white border border-gray-200 rounded-lg">
        <div className="flex items-center gap-1">
          {PERIODS.map(p => (
            <button
              key={p.key}
              onClick={() => setPeriod(p.key)}
              className={`px-3 py-1.5 rounded text-sm font-medium transition-colors ${
                period === p.key
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-3 ml-auto text-sm">
          {items.length > unread.length && (
            <button
              onClick={() => setShowRead(s => !s)}
              className="text-gray-400 hover:text-gray-600 transition-colors"
            >
              {showRead ? 'Скрыть прочитанные' : `+ ${items.length - unread.length} прочитанных`}
            </button>
          )}
          <label className="text-gray-500">Min score</label>
          <input
            type="number"
            min={0} max={1} step={0.1}
            value={minScore}
            onChange={e => setMinScore(parseFloat(e.target.value) || 0)}
            className="w-20 border border-gray-300 rounded px-2 py-1 text-sm"
          />
        </div>
      </div>

      {/* Stats */}
      {!loading && items.length > 0 && (() => {
        const latest = items.reduce((a, b) =>
          new Date(a.publishedAt) > new Date(b.publishedAt) ? a : b)
        const minsAgo = Math.round((Date.now() - new Date(latest.publishedAt).getTime()) / 60000)
        const freshness = minsAgo < 60
          ? `${minsAgo} мин назад`
          : minsAgo < 1440
            ? `${Math.round(minsAgo / 60)} ч назад`
            : `${Math.round(minsAgo / 1440)} д назад`
        return (
          <p className="text-xs text-gray-400 mb-3">
            {unread.length > 0
              ? `${unread.length} непрочитанных · `
              : 'Всё прочитано · '}
            {new Set(items.map(i => i.sourceId)).size} источников · последнее {freshness}
          </p>
        )
      })()}

      {error && (
        <div className="bg-red-50 text-red-700 border border-red-200 rounded p-3 mb-4 text-sm">{error}</div>
      )}

      {!loading && visible.length === 0 && !error && (
        <div className="text-center text-gray-400 py-16">
          {unread.length === 0 && items.length > 0 ? (
            <>
              <p className="text-lg">Всё прочитано.</p>
              <button
                onClick={() => setShowRead(true)}
                className="mt-2 text-sm text-blue-500 hover:underline"
              >
                Показать прочитанные
              </button>
            </>
          ) : (
            <>
              <p className="text-lg">Постов нет.</p>
              <p className="text-sm mt-1">
                {period !== 'all' ? 'Попробуй другой период.' : 'Добавь источники и дождись сбора.'}
              </p>
            </>
          )}
        </div>
      )}

      <div className="space-y-3">
        {visible.map(item => (
          <FeedCard
            key={item.postId}
            item={item}
            sources={sources}
            isRead={readIds.has(item.postId)}
            onRead={markRead}
            onReact={handleReact}
          />
        ))}
      </div>

      {nextToken && (
        <div className="mt-4 text-center">
          <button
            onClick={() => load(nextToken)}
            disabled={loading}
            className="px-4 py-2 bg-gray-100 hover:bg-gray-200 text-gray-700 rounded text-sm disabled:opacity-50"
          >
            {loading ? 'Loading…' : 'Load more'}
          </button>
        </div>
      )}

      {loading && items.length === 0 && (
        <div className="text-center text-gray-400 py-16">Loading…</div>
      )}
    </div>
  )
}
