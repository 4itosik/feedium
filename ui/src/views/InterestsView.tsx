import { useState, useEffect, useRef } from 'react'
import { listInterests, createInterest, deleteInterest, type Interest } from '../api'

export default function InterestsView() {
  const [interests, setInterests] = useState<Interest[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [topic, setTopic] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await listInterests()
      setInterests(res.items ?? [])
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    const t = topic.trim()
    if (!t) return
    setSubmitting(true)
    setError(null)
    try {
      const res = await createInterest(t)
      setInterests(prev => [...prev, res.interest])
      setTopic('')
      inputRef.current?.focus()
    } catch (e) {
      setError(String(e))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteInterest(id)
      setInterests(prev => prev.filter(i => i.id !== id))
    } catch (e) {
      setError(String(e))
    }
  }

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-4">Interests</h1>

      {error && (
        <div className="bg-red-50 text-red-700 border border-red-200 rounded p-3 mb-4 text-sm">{error}</div>
      )}

      <form onSubmit={handleAdd} className="flex gap-2 mb-6">
        <input
          ref={inputRef}
          type="text"
          value={topic}
          onChange={e => setTopic(e.target.value)}
          placeholder="Add interest topic…"
          className="flex-1 border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        <button
          type="submit"
          disabled={submitting || !topic.trim()}
          className="px-4 py-2 bg-blue-600 text-white rounded text-sm hover:bg-blue-700 disabled:opacity-50"
        >
          {submitting ? 'Adding…' : 'Add'}
        </button>
      </form>

      {loading && interests.length === 0 && (
        <div className="text-center text-gray-400 py-12">Loading…</div>
      )}

      {!loading && interests.length === 0 && !error && (
        <div className="text-center text-gray-400 py-12">
          <p>No interests yet.</p>
          <p className="text-sm mt-1">Add topics you care about — they'll be used for scoring.</p>
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        {interests.map(interest => (
          <div
            key={interest.id}
            className="flex items-center gap-1.5 bg-blue-50 border border-blue-200 text-blue-800 rounded-full px-3 py-1 text-sm"
          >
            <span>{interest.topic}</span>
            <button
              onClick={() => handleDelete(interest.id)}
              className="text-blue-400 hover:text-red-600 transition-colors leading-none"
              title="Remove"
            >
              ×
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
