import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatSize, initials, setBrand, shortDate, themeFor, toast, toasts } from './state'
import type { Branding } from './api'

describe('initials', () => {
  it('uses the first letters of name parts or the address', () => {
    expect(initials('Marina Gonçalves')).toBe('MG')
    expect(initials('ana@roosty.test')).toBe('AR')
    expect(initials('"Suporte"')).toBe('S')
    expect(initials('')).toBe('?')
  })
})

describe('formatSize', () => {
  it('formats bytes, KB and MB in pt-BR', () => {
    expect(formatSize(500)).toBe('500 B')
    expect(formatSize(48 * 1024)).toBe('48 KB')
    expect(formatSize(2.5 * 1024 * 1024)).toBe('2,5 MB')
  })
})

describe('shortDate', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-01T15:00:00'))
  })
  afterEach(() => vi.useRealTimers())

  it('shows the time for today, "Ontem" and short dates', () => {
    expect(shortDate(new Date('2026-10-01T09:42:00').toISOString())).toBe('09:42')
    expect(shortDate(new Date('2026-09-30T20:00:00').toISOString())).toBe('Ontem')
    expect(shortDate(new Date('2026-09-27T10:00:00').toISOString())).toMatch(/^27 de set/)
    expect(shortDate(new Date('2025-03-02T10:00:00').toISOString())).toMatch(/2025/)
  })
})

describe('themeFor', () => {
  const brand = (themes: string[], def: string) =>
    setBrand({ name: 'X', accent: '', defaultTheme: def, themes, loginTitle: '', loginMessage: '', links: { support: '', privacy: '' }, customCss: '', logos: [], version: 't', setupNeeded: false } as Branding)

  it('respects the instance list of allowed themes', () => {
    brand(['claro', 'bege'], 'bege')
    expect(themeFor({ theme: 'navy' }).theme).toBe('bege')
    expect(themeFor({ theme: 'claro' }).theme).toBe('claro')
    expect(themeFor({}).theme).toBe('bege')
  })

  it('prefers the user accent over the instance accent', () => {
    brand(['claro'], 'claro')
    expect(themeFor({ accent: '#3D63DD' }).accent).toBe('#3D63DD')
  })
})

describe('toast', () => {
  it('adds a message and removes it after the timeout', () => {
    vi.useFakeTimers()
    toast('Mensagem enviada')
    expect(toasts().some((t) => t.text === 'Mensagem enviada')).toBe(true)
    vi.advanceTimersByTime(5000)
    expect(toasts().some((t) => t.text === 'Mensagem enviada')).toBe(false)
    vi.useRealTimers()
  })
})
