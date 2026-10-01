export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: { 'X-Roosty': '1' }, credentials: 'same-origin' }
  if (body instanceof FormData) init.body = body
  else if (body !== undefined) {
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  let res: Response
  try {
    res = await fetch(url, init)
  } catch {
    throw new ApiError(0, 'Sem conexão com o servidor. Verifique sua internet.')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    if (res.status === 401 && !url.includes('/login') && !url.startsWith('/api/admin') && !url.startsWith('/api/setup')) {
      onUnauthorized?.()
    }
    throw new ApiError(res.status, (data as { error?: string }).error || `Erro ${res.status}`)
  }
  return data as T
}

let onUnauthorized: (() => void) | undefined
export const setUnauthorizedHandler = (fn: () => void) => (onUnauthorized = fn)

export const api = {
  get: <T>(url: string) => request<T>('GET', url),
  post: <T>(url: string, body?: unknown) => request<T>('POST', url, body ?? {}),
  put: <T>(url: string, body?: unknown) => request<T>('PUT', url, body),
  del: <T>(url: string) => request<T>('DELETE', url),
}

type Call = [string, Record<string, unknown>, string]

/** Sends a JMAP-style batch and returns results by tag. Method errors throw. */
export async function batch(calls: Call[]): Promise<Record<string, any>> {
  const res = await api.post<{ results: [string, any, string][] }>('/api', { calls })
  const out: Record<string, any> = {}
  for (const [, result, tag] of res.results) {
    if (result && typeof result === 'object' && 'error' in result) throw new ApiError(400, result.error)
    out[tag] = result
  }
  return out
}

// ---------- types shared with the server ----------

export interface Address { name?: string; email: string }
export interface Mailbox { id: string; name: string; role?: string; parent?: string; total: number; unread: number }
export interface Summary {
  id: number; mailbox: string; from: Address[]; to: Address[]; subject: string; date: string; preview: string
  unread: boolean; flagged: boolean; answered: boolean; hasAttachment: boolean; size: number; messageId?: string
}
export interface Attachment { index: number; name: string; type: string; size: number; inline: boolean }
export interface Message {
  subject: string; from: Address[]; to: Address[]; cc: Address[]; replyTo: Address[]; date: string
  messageId: string; references: string; text: string; attachments: Attachment[]; flagged: boolean
}
export interface Body { message: Message; html: string; hasHtml: boolean; blocked: number; trackers: number; remoteAllowed: boolean }
export interface Branding {
  name: string; accent: string; defaultTheme: string; themes: string[]; loginTitle: string; loginMessage: string
  links: { support: string; privacy: string }; customCss: string; logos: string[] | null; version: string; setupNeeded: boolean
}
export interface Upload { id: string; name: string; type: string; size: number }

export function upload(file: File): Promise<Upload> {
  const fd = new FormData()
  fd.append('file', file)
  return api.post<Upload>('/api/upload', fd)
}
