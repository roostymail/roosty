import { batch as solidBatch, createSignal } from 'solid-js'
import { batch, Body, Mailbox, Summary } from '../../lib/api'
import { toast } from '../../lib/state'

export interface View { mailbox: string; flagged?: boolean; unread?: boolean; text?: string; title: string }

const PAGE = 50

export const [mailboxes, setMailboxes] = createSignal<Mailbox[]>([])
export const [view, setView] = createSignal<View>({ mailbox: 'INBOX', title: 'Entrada' })
export const [messages, setMessages] = createSignal<Summary[]>([])
export const [total, setTotal] = createSignal(0)
export const [loading, setLoading] = createSignal(false)
export const [selected, setSelected] = createSignal<Summary | null>(null)
export const [body, setBody] = createSignal<Body | null>(null)
export const [bodyLoading, setBodyLoading] = createSignal(false)
export const [screen, setScreen] = createSignal<'mail' | 'settings'>('mail')
export const [drawer, setDrawer] = createSignal(false)

export const roleBox = (role: string) => mailboxes().find((m) => m.role === role)

export const ROLE_LABEL: Record<string, string> = {
  inbox: 'Entrada', drafts: 'Rascunhos', sent: 'Enviados', archive: 'Arquivo', junk: 'Spam', trash: 'Lixeira',
}
export const ROLE_ICON: Record<string, string> = {
  inbox: 'inbox', drafts: 'file', sent: 'send', archive: 'archive', junk: 'alert', trash: 'trash',
}

export async function loadMailboxes() {
  try {
    const r = await batch([['Mailbox/get', {}, 'm']])
    setMailboxes(r.m.list)
  } catch (e) {
    toast((e as Error).message, 'error')
  }
}

export async function loadMessages(append = false) {
  const v = view()
  setLoading(true)
  try {
    const position = append ? messages().length : 0
    const filter: Record<string, unknown> = {}
    if (v.flagged) filter.flagged = true
    if (v.unread) filter.unread = true
    if (v.text) filter.text = v.text
    const r = await batch([
      ['Email/query', { mailbox: v.mailbox, filter, position, limit: PAGE }, 'q'],
      ['Email/get', { mailbox: v.mailbox, '#ids': { resultOf: 'q', path: '/ids' } }, 'g'],
    ])
    if (view() !== v) return
    solidBatch(() => {
      setTotal(r.q.total)
      setMessages(append ? [...messages(), ...r.g.list] : r.g.list)
      const sel = selected()
      if (sel && !append) setSelected(r.g.list.find((m: Summary) => m.id === sel.id && m.mailbox === sel.mailbox) || sel)
    })
  } catch (e) {
    toast((e as Error).message, 'error')
  } finally {
    setLoading(false)
  }
}

export function openView(v: View) {
  solidBatch(() => {
    setView(v)
    setSelected(null)
    setBody(null)
    setMessages([])
    setScreen('mail')
    setDrawer(false)
  })
  loadMessages()
}

export async function openMessage(m: Summary, allowRemote = false) {
  setSelected(m)
  setBodyLoading(true)
  if (!allowRemote) setBody(null)
  try {
    const r = await batch([['Email/body', { mailbox: m.mailbox, id: m.id, allowRemote }, 'b']])
    if (selected()?.id !== m.id) return
    setBody(r.b)
    if (m.unread) {
      patchMessage(m.id, { unread: false })
      bumpUnread(m.mailbox, -1)
    }
  } catch (e) {
    toast((e as Error).message, 'error')
  } finally {
    setBodyLoading(false)
  }
}

export function closeMessage() {
  setSelected(null)
  setBody(null)
}

function patchMessage(id: number, patch: Partial<Summary>) {
  setMessages((l) => l.map((m) => (m.id === id ? { ...m, ...patch } : m)))
  const s = selected()
  if (s?.id === id) setSelected({ ...s, ...patch })
}

function bumpUnread(mailbox: string, delta: number) {
  setMailboxes((l) => l.map((b) => (b.id === mailbox ? { ...b, unread: Math.max(0, b.unread + delta) } : b)))
}

export async function setFlag(m: Summary, key: 'seen' | 'flagged', value: boolean) {
  const patch = key === 'seen' ? { unread: !value } : { flagged: value }
  patchMessage(m.id, patch)
  if (key === 'seen') bumpUnread(m.mailbox, value ? -1 : 1)
  try {
    await batch([['Email/set', { mailbox: m.mailbox, ids: [m.id], [key]: value }, 's']])
  } catch (e) {
    toast((e as Error).message, 'error')
    loadMessages()
  }
}

/** Removes a message from the list and selects the next one. */
function dropFromList(m: Summary) {
  const list = messages()
  const i = list.findIndex((x) => x.id === m.id)
  const next = list[i + 1] || list[i - 1] || null
  setMessages(list.filter((x) => x.id !== m.id))
  setTotal((t) => Math.max(0, t - 1))
  if (selected()?.id === m.id) {
    if (next && window.innerWidth > 900) openMessage(next)
    else closeMessage()
  }
}

export async function moveTo(m: Summary, role: string, label: string) {
  dropFromList(m)
  try {
    const args: Record<string, unknown> = { mailbox: m.mailbox, ids: [m.id] }
    if (role === 'destroy') args.destroy = true
    else args.moveToRole = role
    await batch([['Email/set', args, 's']])
    toast(label)
    loadMailboxes()
  } catch (e) {
    toast((e as Error).message, 'error')
    loadMessages()
  }
}
