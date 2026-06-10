// Proto JSON transcoding: proto snake_case → JSON camelCase.

export interface FeedItem {
  postId: string
  sourceId: string
  author: string
  text: string
  publishedAt: string
  score: number
}

export interface FeedResponse {
  items: FeedItem[]
  nextPageToken: string
}

export type SourceType =
  | 'SOURCE_TYPE_TELEGRAM_CHANNEL'
  | 'SOURCE_TYPE_TELEGRAM_GROUP'
  | 'SOURCE_TYPE_RSS'
  | 'SOURCE_TYPE_HTML'

export interface TelegramChannelConfig { tgId: string; username: string }
export interface TelegramGroupConfig   { tgId: string; username: string }
export interface RSSConfig             { feedUrl: string }
export interface HTMLConfig            { url: string }

export type SourceConfig =
  | { telegramChannel: TelegramChannelConfig }
  | { telegramGroup: TelegramGroupConfig }
  | { rss: RSSConfig }
  | { html: HTMLConfig }

export interface Source {
  id: string
  type: SourceType
  processingMode: string
  config: SourceConfig
  createdAt: string
  updatedAt: string
}

export interface SourcesResponse {
  items: Source[]
  nextPageToken: string
}

export interface Interest {
  id: string
  topic: string
  model: string
  createdAt: string
  updatedAt: string
}

export interface InterestsResponse {
  items: Interest[]
  nextPageToken: string
}

export interface PostSourceRef {
  id: string
  type: string
}

export interface Post {
  id: string
  source: PostSourceRef
  externalId: string
  publishedAt: string
  author?: string
  text: string
  createdAt: string
  updatedAt: string
}

export interface PostsResponse {
  items: Post[]
  nextPageToken: string
}

export interface Summary {
  id: string
  postId?: string
  sourceId: string
  text: string
  wordCount: number
  createdAt: string
}

export interface SummariesResponse {
  summaries: Summary[]
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    ...init,
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`${res.status} ${res.statusText}: ${body}`)
  }
  return res.json() as Promise<T>
}

// Feed
export function listFeed(params?: {
  minScore?: number
  from?: string
  to?: string
  pageSize?: number
  pageToken?: string
}): Promise<FeedResponse> {
  const q = new URLSearchParams()
  if (params?.minScore !== undefined) q.set('min_score', String(params.minScore))
  if (params?.from) q.set('from', params.from)
  if (params?.to) q.set('to', params.to)
  if (params?.pageSize) q.set('page_size', String(params.pageSize))
  if (params?.pageToken) q.set('page_token', params.pageToken)
  const qs = q.toString()
  return apiFetch<FeedResponse>(`/v1/feed${qs ? '?' + qs : ''}`)
}

// Sources
export function listSources(pageToken?: string): Promise<SourcesResponse> {
  const q = new URLSearchParams({ page_size: '100' })
  if (pageToken) q.set('page_token', pageToken)
  return apiFetch<SourcesResponse>(`/v1/sources?${q}`)
}

export function createSource(body: {
  type: SourceType
  config: SourceConfig
}): Promise<{ source: Source }> {
  return apiFetch('/v1/sources', { method: 'POST', body: JSON.stringify(body) })
}

export function deleteSource(id: string): Promise<unknown> {
  return apiFetch(`/v1/sources/${id}`, { method: 'DELETE' })
}

// Interests
export function listInterests(): Promise<InterestsResponse> {
  return apiFetch<InterestsResponse>('/v1/interests?page_size=100')
}

export function createInterest(topic: string): Promise<{ interest: Interest }> {
  return apiFetch('/v1/interests', { method: 'POST', body: JSON.stringify({ topic }) })
}

export function deleteInterest(id: string): Promise<unknown> {
  return apiFetch(`/v1/interests/${id}`, { method: 'DELETE' })
}

export type PostSortField = 'POST_SORT_FIELD_PUBLISHED_AT' | 'POST_SORT_FIELD_CREATED_AT'
export type SortDirection = 'SORT_DIRECTION_DESC' | 'SORT_DIRECTION_ASC'

// Posts
export function listPosts(params?: {
  pageSize?: number
  pageToken?: string
  sourceId?: string
  orderBy?: PostSortField
  orderDir?: SortDirection
}): Promise<PostsResponse> {
  const q = new URLSearchParams({ page_size: String(params?.pageSize ?? 50) })
  if (params?.pageToken) q.set('page_token', params.pageToken)
  if (params?.sourceId) q.set('source_id', params.sourceId)
  if (params?.orderBy) q.set('order_by', params.orderBy)
  if (params?.orderDir) q.set('order_dir', params.orderDir)
  return apiFetch<PostsResponse>(`/v1/posts?${q}`)
}

export function listPostSummaries(postId: string): Promise<SummariesResponse> {
  return apiFetch<SummariesResponse>(`/v1/posts/${postId}/summaries`)
}
