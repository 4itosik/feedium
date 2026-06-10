import { useState, useEffect } from 'react'
import { listPosts, listPostSummaries, type Post, type Summary } from '../api'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

function PostRow({ post }: { post: Post }) {
  const [open, setOpen] = useState(false)
  const [summaries, setSummaries] = useState<Summary[]>([])
  const [loadingSummaries, setLoadingSummaries] = useState(false)
  const [summaryError, setSummaryError] = useState<string | null>(null)

  const handleToggle = async () => {
    if (!open && summaries.length === 0) {
      setLoadingSummaries(true)
      setSummaryError(null)
      try {
        const res = await listPostSummaries(post.id)
        setSummaries(res.summaries ?? [])
      } catch (e) {
        setSummaryError(String(e))
      } finally {
        setLoadingSummaries(false)
      }
    }
    setOpen(o => !o)
  }

  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden">
      <button
        onClick={handleToggle}
        className="w-full text-left px-4 py-3 hover:bg-gray-50 transition-colors"
      >
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2 text-sm text-gray-500 min-w-0">
            {post.author && <span className="font-medium text-gray-800 truncate">{post.author}</span>}
            <span className="shrink-0">{formatDate(post.publishedAt)}</span>
            <span className="text-xs text-gray-400 font-mono truncate hidden sm:block">{post.source.id}</span>
          </div>
          <span className="text-gray-400 shrink-0">{open ? '▲' : '▼'}</span>
        </div>
        <p className="mt-1 text-sm text-gray-700 line-clamp-2 text-left">{post.text}</p>
      </button>

      {open && (
        <div className="border-t border-gray-100 px-4 py-3 bg-gray-50">
          <p className="text-sm text-gray-700 whitespace-pre-wrap mb-3">{post.text}</p>

          <div className="border-t border-gray-200 pt-3">
            <h3 className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">Summaries</h3>

            {loadingSummaries && (
              <p className="text-sm text-gray-400">Loading summaries…</p>
            )}

            {summaryError && (
              <p className="text-sm text-red-600">{summaryError}</p>
            )}

            {!loadingSummaries && !summaryError && summaries.length === 0 && (
              <p className="text-sm text-gray-400">No summaries yet.</p>
            )}

            {summaries.map(s => (
              <div key={s.id} className="mb-2 last:mb-0">
                <p className="text-sm text-gray-800">{s.text}</p>
                <p className="text-xs text-gray-400 mt-0.5">{s.wordCount} words · {formatDate(s.createdAt)}</p>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

export default function PostsView() {
  const [posts, setPosts] = useState<Post[]>([])
  const [nextToken, setNextToken] = useState<string | undefined>()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = async (token?: string) => {
    setLoading(true)
    setError(null)
    try {
      const res = await listPosts({ pageSize: 50, pageToken: token })
      setPosts(prev => token ? [...prev, ...(res.items ?? [])] : (res.items ?? []))
      setNextToken(res.nextPageToken || undefined)
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-bold text-gray-900">Posts</h1>
        <button
          onClick={() => load()}
          className="px-3 py-1.5 bg-gray-100 hover:bg-gray-200 text-gray-700 rounded text-sm"
        >
          Refresh
        </button>
      </div>

      {error && (
        <div className="bg-red-50 text-red-700 border border-red-200 rounded p-3 mb-4 text-sm">{error}</div>
      )}

      {loading && posts.length === 0 && (
        <div className="text-center text-gray-400 py-16">Loading…</div>
      )}

      {!loading && posts.length === 0 && !error && (
        <div className="text-center text-gray-400 py-16">
          <p>No posts yet.</p>
          <p className="text-sm mt-1">Posts will appear here once collectors run.</p>
        </div>
      )}

      <div className="space-y-2">
        {posts.map(post => <PostRow key={post.id} post={post} />)}
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
    </div>
  )
}
