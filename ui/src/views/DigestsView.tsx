import { useState, useEffect, useCallback } from 'react'
import { listSources, listSourceSummaries, type Source, type Summary } from '../api'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

function sourceLabel(source: Source): string {
  const cfg = source.config as Record<string, unknown>
  if (cfg.telegramChannel) return `@${(cfg.telegramChannel as { username?: string }).username ?? source.id}`
  if (cfg.telegramGroup)   return `@${(cfg.telegramGroup as { username?: string }).username ?? source.id}`
  if (cfg.rss)             return (cfg.rss as { feedUrl?: string }).feedUrl ?? source.id
  if (cfg.html)            return (cfg.html as { url?: string }).url ?? source.id
  return source.id
}

interface Digest {
  summary: Summary
  source: Source
}

function DigestCard({ digest }: { digest: Digest }) {
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden px-4 py-3">
      <div className="flex items-center justify-between gap-3 text-sm text-gray-500 mb-1">
        <span className="font-medium text-gray-800 truncate">{sourceLabel(digest.source)}</span>
        <span className="shrink-0">{formatDate(digest.summary.createdAt)}</span>
      </div>
      <p className="text-sm text-gray-700 whitespace-pre-wrap">{digest.summary.text}</p>
      <p className="text-xs text-gray-400 mt-1">{digest.summary.wordCount} words</p>
    </div>
  )
}

export default function DigestsView() {
  const [digests, setDigests] = useState<Digest[]>([])
  const [sourceCount, setSourceCount] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const sourcesRes = await listSources()
      const cumulative = (sourcesRes.items ?? []).filter(
        s => s.processingMode === 'PROCESSING_MODE_CUMULATIVE',
      )
      setSourceCount(cumulative.length)

      const perSource = await Promise.all(
        cumulative.map(source =>
          listSourceSummaries(source.id, { pageSize: 50 })
            .then(res => (res.summaries ?? []).map(summary => ({ summary, source })))
            .catch(() => [] as Digest[]),
        ),
      )

      const merged = perSource.flat().sort(
        (a, b) => new Date(b.summary.createdAt).getTime() - new Date(a.summary.createdAt).getTime(),
      )
      setDigests(merged)
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h1 className="text-2xl font-bold text-gray-900">Digests</h1>
        <button
          onClick={() => load()}
          className="px-3 py-1.5 bg-gray-100 hover:bg-gray-200 text-gray-700 rounded text-sm"
        >
          Refresh
        </button>
      </div>

      <p className="text-xs text-gray-400 mb-4">
        Periodic digests of cumulative sources (group chats) — aggregated text, not individually scored.
      </p>

      {error && (
        <div className="bg-red-50 text-red-700 border border-red-200 rounded p-3 mb-4 text-sm">{error}</div>
      )}

      {loading && digests.length === 0 && (
        <div className="text-center text-gray-400 py-16">Loading…</div>
      )}

      {!loading && !error && sourceCount === 0 && (
        <div className="text-center text-gray-400 py-16">
          <p>No cumulative sources configured.</p>
        </div>
      )}

      {!loading && !error && sourceCount > 0 && digests.length === 0 && (
        <div className="text-center text-gray-400 py-16">
          <p>No digests yet.</p>
          <p className="text-sm mt-1">Digests appear after the next scheduled summarization cycle.</p>
        </div>
      )}

      <div className="space-y-2">
        {digests.map(d => <DigestCard key={d.summary.id} digest={d} />)}
      </div>
    </div>
  )
}
