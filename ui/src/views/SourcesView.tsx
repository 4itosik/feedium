import { useState, useEffect } from 'react'
import { listSources, createSource, deleteSource, type Source, type SourceType } from '../api'

const SOURCE_TYPE_LABELS: Record<SourceType, string> = {
  SOURCE_TYPE_TELEGRAM_CHANNEL: 'Telegram Channel',
  SOURCE_TYPE_TELEGRAM_GROUP:   'Telegram Group',
  SOURCE_TYPE_RSS:              'RSS',
  SOURCE_TYPE_HTML:             'Web (HTML)',
}

function sourceConfigSummary(source: Source): string {
  const cfg = source.config as Record<string, unknown>
  if (cfg.telegramChannel) {
    const c = cfg.telegramChannel as { username?: string }
    return `@${c.username ?? ''}`
  }
  if (cfg.telegramGroup) {
    const c = cfg.telegramGroup as { username?: string }
    return `@${c.username ?? ''}`
  }
  if (cfg.rss) {
    const c = cfg.rss as { feedUrl?: string }
    return c.feedUrl ?? ''
  }
  if (cfg.html) {
    const c = cfg.html as { url?: string }
    return c.url ?? ''
  }
  return ''
}

const EMPTY_FORM = {
  type: 'SOURCE_TYPE_RSS' as SourceType,
  rssUrl: '',
  htmlUrl: '',
  tgUsername: '',
}

export default function SourcesView() {
  const [sources, setSources] = useState<Source[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await listSources()
      setSources(res.items ?? [])
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      let config
      if (form.type === 'SOURCE_TYPE_RSS') {
        config = { rss: { feedUrl: form.rssUrl } }
      } else if (form.type === 'SOURCE_TYPE_HTML') {
        config = { html: { url: form.htmlUrl } }
      } else if (form.type === 'SOURCE_TYPE_TELEGRAM_CHANNEL') {
        config = { telegramChannel: { tgId: '0', username: form.tgUsername } }
      } else {
        config = { telegramGroup: { tgId: '0', username: form.tgUsername } }
      }
      await createSource({ type: form.type, config })
      setForm(EMPTY_FORM)
      setShowForm(false)
      await load()
    } catch (e) {
      setError(String(e))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this source?')) return
    try {
      await deleteSource(id)
      setSources(prev => prev.filter(s => s.id !== id))
    } catch (e) {
      setError(String(e))
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-bold text-gray-900">Sources</h1>
        <button
          onClick={() => setShowForm(f => !f)}
          className="px-3 py-1.5 bg-blue-600 text-white rounded text-sm hover:bg-blue-700"
        >
          {showForm ? 'Cancel' : '+ Add Source'}
        </button>
      </div>

      {error && (
        <div className="bg-red-50 text-red-700 border border-red-200 rounded p-3 mb-4 text-sm">{error}</div>
      )}

      {showForm && (
        <form onSubmit={handleCreate} className="bg-white border border-gray-200 rounded-lg p-4 mb-4 space-y-3">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
            <select
              value={form.type}
              onChange={e => setForm(f => ({ ...f, type: e.target.value as SourceType }))}
              className="border border-gray-300 rounded px-2 py-1.5 text-sm w-full"
            >
              {(Object.keys(SOURCE_TYPE_LABELS) as SourceType[]).map(t => (
                <option key={t} value={t}>{SOURCE_TYPE_LABELS[t]}</option>
              ))}
            </select>
          </div>

          {form.type === 'SOURCE_TYPE_RSS' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Feed URL</label>
              <input
                required
                type="url"
                value={form.rssUrl}
                onChange={e => setForm(f => ({ ...f, rssUrl: e.target.value }))}
                placeholder="https://example.com/feed.xml"
                className="border border-gray-300 rounded px-2 py-1.5 text-sm w-full"
              />
            </div>
          )}

          {form.type === 'SOURCE_TYPE_HTML' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Page URL</label>
              <input
                required
                type="url"
                value={form.htmlUrl}
                onChange={e => setForm(f => ({ ...f, htmlUrl: e.target.value }))}
                placeholder="https://example.com"
                className="border border-gray-300 rounded px-2 py-1.5 text-sm w-full"
              />
            </div>
          )}

          {(form.type === 'SOURCE_TYPE_TELEGRAM_CHANNEL' || form.type === 'SOURCE_TYPE_TELEGRAM_GROUP') && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Username (without @)</label>
              <input
                required
                type="text"
                value={form.tgUsername}
                onChange={e => setForm(f => ({ ...f, tgUsername: e.target.value }))}
                placeholder="channelname"
                className="border border-gray-300 rounded px-2 py-1.5 text-sm w-full"
              />
            </div>
          )}

          <button
            type="submit"
            disabled={submitting}
            className="px-4 py-1.5 bg-green-600 text-white rounded text-sm hover:bg-green-700 disabled:opacity-50"
          >
            {submitting ? 'Creating…' : 'Create'}
          </button>
        </form>
      )}

      {loading && sources.length === 0 && (
        <div className="text-center text-gray-400 py-12">Loading…</div>
      )}

      {!loading && sources.length === 0 && !error && (
        <div className="text-center text-gray-400 py-12">
          <p>No sources yet.</p>
          <p className="text-sm mt-1">Add your first source above.</p>
        </div>
      )}

      <div className="space-y-2">
        {sources.map(source => (
          <div key={source.id} className="bg-white border border-gray-200 rounded-lg px-4 py-3 flex items-center justify-between">
            <div>
              <span className="text-sm font-medium text-gray-900">
                {SOURCE_TYPE_LABELS[source.type] ?? source.type}
              </span>
              <span className="ml-2 text-sm text-gray-500">{sourceConfigSummary(source)}</span>
              <span className="ml-2 text-xs text-gray-400 font-mono">{source.processingMode}</span>
            </div>
            <button
              onClick={() => handleDelete(source.id)}
              className="text-xs text-red-600 hover:text-red-800 hover:underline"
            >
              Delete
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
