import { describe, expect, it } from 'vitest'
import { ACCENTS, applyTheme, contrast, luminance, THEMES, themeById, validHex } from './theme'

describe('contrast', () => {
  it('matches the WCAG reference values', () => {
    expect(contrast('#000000', '#FFFFFF')).toBeCloseTo(21, 1)
    expect(contrast('#FFFFFF', '#FFFFFF')).toBeCloseTo(1, 5)
    expect(luminance('#FFFFFF')).toBeCloseTo(1, 5)
  })
})

describe('built-in themes', () => {
  it('has the 10 themes with unique ids', () => {
    expect(THEMES).toHaveLength(10)
    expect(new Set(THEMES.map((t) => t.id)).size).toBe(10)
  })

  for (const t of THEMES) {
    it(`${t.name}: readable text, secondary text and accent`, () => {
      expect(contrast(t.text, t.list)).toBeGreaterThanOrEqual(7) // WCAG AAA for body text
      expect(contrast(t.muted, t.list)).toBeGreaterThanOrEqual(4.5) // AA
      expect(contrast(t.accent, t.list)).toBeGreaterThanOrEqual(4.5) // links and accent text
      for (const c of [t.chrome, t.list, t.surface, t.border, t.text, t.muted, t.accent, t.paper]) expect(validHex(c)).toBe(true)
    })
  }

  it('suggested accents are valid colors', () => {
    for (const c of ACCENTS) expect(validHex(c)).toBe(true)
  })

  it('falls back to the first theme for unknown ids', () => {
    expect(themeById('nope').id).toBe('claro')
    expect(themeById('navy').name).toBe('Navy')
  })
})

describe('applyTheme', () => {
  it('sets every variable and picks a readable color on the accent', () => {
    const el = document.createElement('div')
    applyTheme('preto', '#FFD400', el)
    expect(el.style.getPropertyValue('--list')).toBe('#000000')
    expect(el.style.getPropertyValue('--accent')).toBe('#FFD400')
    expect(el.style.getPropertyValue('--on-accent')).toBe('#111111') // dark text on yellow
    applyTheme('claro', '#66753F', el)
    expect(el.style.getPropertyValue('--on-accent')).toBe('#FFFFFF')
  })

  it('ignores an invalid accent and keeps the theme default', () => {
    const el = document.createElement('div')
    applyTheme('navy', 'red;background:url(x)', el)
    expect(el.style.getPropertyValue('--accent')).toBe('#F2B84B')
  })

  it('rejects hex values that are not #RRGGBB', () => {
    for (const v of ['#fff', 'fff000', '#GGGGGG', '#12345', '#1234567', 'javascript:x']) expect(validHex(v)).toBe(false)
  })
})
