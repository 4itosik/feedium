import { useState, useEffect, useCallback } from 'react'
import { listFeed, type FeedItem } from '../api'

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

function FeedCard({ item }: { item: FeedItem }) {
  const [expanded, setExpanded] = useState(false)
  const preview = item.text.slice(0, 280)
  const hasMore = item.text.length > 280

  return (
    <div className="bg-white rounded-lg border border-gray-200 p-4 shadow-sm hover:shadow-md transition-shadow">
      <div className="flex items-start justify-between gap-3 mb-2">
        <div className="flex items-center gap-2 text-sm text-gray-500">
          {item.author && <span className="font-medium text-gray-700">{item.author}</span>}
          <span>{formatDate(item.publishedAt)}</span>
          <span className="text-xs text-gray-400 font-mono truncate max-w-32">{item.sourceId}</span>
        </div>
        <span className={`text-xs font-semibold px-2 py-0.5 rounded-full shrink-0 ${scoreColor(item.score)}`}>
          {item.score.toFixed(2)}
        </span>
      </div>
      <p className="text-gray-800 text-sm leading-relaxed whitespace-pre-wrap">
        {expanded ? item.text : preview}
        {!expanded && hasMore && '…'}
      </p>
      {hasMore && (
        <button
          onClick={() => setExpanded(e => !e)}
          className="mt-1 text-xs text-blue-600 hover:underline"
        >
          {expanded ? 'Show less' : 'Show more'}
        </button>
      )}
    </div>
  )
}

export default function FeedView() {
  const [items, setItems] = useState<FeedItem[]>([])
  const [nextToken, setNextToken] = useState<string | undefined>()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [minScore, setMinScore] = useState(0)

  const load = useCallback(async (token?: string) => {
    setLoading(true)
    setError(null)
    try {
      const res = await listFeed({ minScore: minScore || undefined, pageSize: 50, pageToken: token })
      setItems(prev => token ? [...prev, ...(res.items ?? [])] : (res.items ?? []))
      setNextToken(res.nextPageToken || undefined)
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }, [minScore])

  useEffect(() => { load() }, [load])

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-bold text-gray-900">Feed</h1>
        <div className="flex items-center gap-2 text-sm">
          <label className="text-gray-600">Min score</label>
          <input
            type="number"
            min={0} max={1} step={0.1}
            value={minScore}
            onChange={e => setMinScore(parseFloat(e.target.value) || 0)}
            className="w-20 border border-gray-300 rounded px-2 py-1 text-sm"
          />
          <button
            onClick={() => load()}
            className="px-3 py-1 bg-blue-600 text-white rounded text-sm hover:bg-blue-700"
          >
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div className="bg-red-50 text-red-700 border border-red-200 rounded p-3 mb-4 text-sm">{error}</div>
      )}

      {!loading && items.length === 0 && !error && (
        <div className="text-center text-gray-400 py-16">
          <p className="text-lg">No items in feed.</p>
          <p className="text-sm mt-1">Add sources and wait for the collectors to run.</p>
        </div>
      )}

      <div className="space-y-3">
        {items.map(item => <FeedCard key={item.postId} item={item} />)}
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
