import { createSignal } from 'solid-js'
import { api, Branding } from './api'
import { applyTheme } from './theme'

export interface Prefs {
  theme?: string
  accent?: string
  density?: 'comfortable' | 'compact'
  displayName?: string
  signature?: string
  trustedSenders?: string[]
  fontSize?: 'small' | 'normal' | 'large'
}

export const [brand, setBrand] = createSignal<Branding | null>(null)
export const [me, setMe] = createSignal<{ email: string; prefs: Prefs } | null>(null)

export async function loadBranding() {
  try {
    const b = await api.get<Branding>('/api/branding')
    setBrand(b)
    if (b.customCss) {
      const st = document.createElement('style')
      st.textContent = b.customCss
      document.head.appendChild(st)
    }
    if (!me()) applyTheme(b.defaultTheme || 'claro', b.accent)
    document.title = b.name || 'Roosty Mail'
  } catch {
    applyTheme('claro')
  }
}

export function themeFor(prefs?: Prefs) {
  const b = brand()
  const allowed = b?.themes?.length ? b.themes : null
  let t = prefs?.theme || b?.defaultTheme || 'claro'
  if (allowed && !allowed.includes(t)) t = b?.defaultTheme || allowed[0]
  return { theme: t, accent: prefs?.accent || b?.accent || '' }
}

export function applyUserTheme() {
  const p = me()?.prefs
  const { theme, accent } = themeFor(p)
  applyTheme(theme, accent)
  const root = document.documentElement
  root.dataset.density = p?.density || 'comfortable'
  root.dataset.fontSize = p?.fontSize || 'normal'
}

let saveTimer: number | undefined
export function updatePrefs(patch: Partial<Prefs>) {
  const cur = me()
  if (!cur) return
  const prefs = { ...cur.prefs, ...patch }
  setMe({ ...cur, prefs })
  applyUserTheme()
  clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    api.put('/api/prefs', prefs).catch((e) => toast(e.message, 'error'))
  }, 400)
}

// ---------- toasts ----------

export interface Toast { id: number; text: string; kind: 'info' | 'error'; action?: { label: string; run: () => void } }
export const [toasts, setToasts] = createSignal<Toast[]>([])
let toastId = 0
export function toast(text: string, kind: Toast['kind'] = 'info', action?: Toast['action'], ms = 4500) {
  const t = { id: ++toastId, text, kind, action }
  setToasts((l) => [...l, t])
  setTimeout(() => setToasts((l) => l.filter((x) => x.id !== t.id)), ms)
}

export function initials(name: string) {
  const parts = name.replace(/[<>"']/g, '').trim().split(/[\s@._-]+/).filter(Boolean)
  return ((parts[0]?.[0] || '?') + (parts[1]?.[0] || '')).toUpperCase()
}

const rtf = new Intl.DateTimeFormat('pt-BR', { hour: '2-digit', minute: '2-digit' })
const dfShort = new Intl.DateTimeFormat('pt-BR', { day: 'numeric', month: 'short' })
const dfYear = new Intl.DateTimeFormat('pt-BR', { day: 'numeric', month: 'short', year: 'numeric' })
const dfFull = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'medium', timeStyle: 'short' })

export function shortDate(iso: string) {
  const d = new Date(iso)
  const now = new Date()
  if (d.toDateString() === now.toDateString()) return rtf.format(d)
  const y = new Date(now)
  y.setDate(now.getDate() - 1)
  if (d.toDateString() === y.toDateString()) return 'Ontem'
  return (d.getFullYear() === now.getFullYear() ? dfShort : dfYear).format(d).replace('.', '')
}
export const fullDate = (iso: string) => dfFull.format(new Date(iso))

export function formatSize(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`
  return `${(n / 1024 / 1024).toFixed(1).replace('.', ',')} MB`
}
