import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, batch, setUnauthorizedHandler } from './api'

const respond = (status: number, body: unknown) =>
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))

afterEach(() => vi.restoreAllMocks())

describe('api', () => {
  it('sends the CSRF header and JSON body on writes', async () => {
    const f = respond(200, { ok: true })
    await api.post('/api/x', { a: 1 })
    const [, init] = f.mock.calls[0]
    expect((init!.headers as Record<string, string>)['X-Roosty']).toBe('1')
    expect((init!.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    expect(init!.body).toBe('{"a":1}')
  })

  it('turns server errors into ApiError with the server message', async () => {
    respond(403, { error: 'este endereço não tem acesso' })
    await expect(api.post('/api/auth/login', {})).rejects.toMatchObject({ status: 403, message: 'este endereço não tem acesso' })
  })

  it('explains network failures in plain language', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('Failed to fetch'))
    await expect(api.get('/api/me')).rejects.toBeInstanceOf(ApiError)
    await expect(api.get('/api/me')).rejects.toThrow(/Sem conexão/)
  })

  it('calls the unauthorized handler on 401, except for login and admin routes', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    respond(401, { error: 'sessão expirada' })
    await api.get('/api/me').catch(() => {})
    expect(handler).toHaveBeenCalledTimes(1)
    await api.post('/api/auth/login', {}).catch(() => {})
    await api.get('/api/admin/me').catch(() => {})
    expect(handler).toHaveBeenCalledTimes(1)
  })
})

describe('batch', () => {
  it('returns results by tag', async () => {
    respond(200, { results: [['Mailbox/get', { list: [] }, 'm'], ['Email/query', { ids: [3], total: 1 }, 'q']] })
    const r = await batch([['Mailbox/get', {}, 'm'], ['Email/query', {}, 'q']])
    expect(r.q.ids).toEqual([3])
  })

  it('throws when a method returns an error', async () => {
    respond(200, { results: [['Email/send', { error: 'adicione pelo menos um destinatário' }, 's']] })
    await expect(batch([['Email/send', {}, 's']])).rejects.toThrow('adicione pelo menos um destinatário')
  })
})
