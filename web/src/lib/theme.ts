// Built-in themes. Same tokens as the Figma variable collection "Tema".
export interface Theme {
  id: string; name: string; dark: boolean
  chrome: string; list: string; surface: string; border: string; text: string; muted: string; accent: string; paper: string
}

export const THEMES: Theme[] = [
  { id: 'claro', name: 'Claro', dark: false, chrome: '#F6F7F9', list: '#FFFFFF', surface: '#EEF0F3', border: '#E3E6EA', text: '#15171A', muted: '#646B76', accent: '#66753F', paper: '#FFFFFF' },
  { id: 'grafite', name: 'Grafite', dark: true, chrome: '#232629', list: '#2A2D31', surface: '#33373C', border: '#3E4349', text: '#E6E8EB', muted: '#9AA1AB', accent: '#A9B67E', paper: '#FFFFFF' },
  { id: 'preto', name: 'Preto', dark: true, chrome: '#0A0A0B', list: '#000000', surface: '#17181A', border: '#232529', text: '#EDEDEF', muted: '#8B8F98', accent: '#A9B67E', paper: '#FFFFFF' },
  { id: 'navy', name: 'Navy', dark: true, chrome: '#0A1424', list: '#0E1A2E', surface: '#16243C', border: '#22334F', text: '#E4EAF4', muted: '#8D9BB4', accent: '#F2B84B', paper: '#FFFFFF' },
  { id: 'roxo', name: 'Roxo', dark: true, chrome: '#150F27', list: '#1B1530', surface: '#251D3E', border: '#33294F', text: '#ECE7F7', muted: '#A398BD', accent: '#B38CFF', paper: '#FFFFFF' },
  { id: 'bege', name: 'Bege', dark: false, chrome: '#EFE9DC', list: '#F7F2EA', surface: '#E8DFD0', border: '#DDD2C0', text: '#2B2620', muted: '#776C5E', accent: '#2A6F5E', paper: '#FFFDF9' },
  { id: 'sepia', name: 'Sépia', dark: false, chrome: '#E9DCBF', list: '#F1E7D0', surface: '#E2D3B2', border: '#D4C29C', text: '#3B2F1E', muted: '#6E5D40', accent: '#8A4B2A', paper: '#FBF5E6' },
  { id: 'contraste', name: 'Alto contraste', dark: true, chrome: '#000000', list: '#000000', surface: '#1A1A1A', border: '#FFFFFF', text: '#FFFFFF', muted: '#E0E0E0', accent: '#FFD400', paper: '#FFFFFF' },
  { id: 'floresta', name: 'Floresta', dark: true, chrome: '#0F1A16', list: '#13201B', surface: '#1C2C25', border: '#293C33', text: '#E3EDE7', muted: '#93A89C', accent: '#6FCF97', paper: '#FFFFFF' },
  { id: 'nevoa', name: 'Névoa', dark: false, chrome: '#E8EDF2', list: '#F3F6F9', surface: '#DDE4EC', border: '#CFD8E2', text: '#1C2530', muted: '#5E6B7A', accent: '#0F7B8A', paper: '#FFFFFF' },
]

export const ACCENTS = ['#66753F', '#2A6F5E', '#3D63DD', '#8A4B2A', '#B4467A', '#7C5CDB', '#C2410C']

const hex = (h: string) => {
  const s = h.replace('#', '')
  return [0, 2, 4].map((i) => parseInt(s.slice(i, i + 2), 16) / 255)
}
export function luminance(h: string) {
  const f = (v: number) => (v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4))
  const [r, g, b] = hex(h).map(f)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}
export function contrast(a: string, b: string) {
  const x = luminance(a), y = luminance(b)
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
}
export const validHex = (h: string) => /^#[0-9a-fA-F]{6}$/.test(h)

export function themeById(id: string) {
  return THEMES.find((t) => t.id === id) || THEMES[0]
}

/** Applies a theme and optional accent override as CSS variables. */
export function applyTheme(id: string, accent?: string, target: HTMLElement = document.documentElement) {
  const t = themeById(id)
  const acc = accent && validHex(accent) ? accent : t.accent
  const on = contrast(acc, '#FFFFFF') >= contrast(acc, '#111111') ? '#FFFFFF' : '#111111'
  const vars: Record<string, string> = {
    '--chrome': t.chrome, '--list': t.list, '--surface': t.surface, '--border': t.border, '--text': t.text,
    '--muted': t.muted, '--accent': acc, '--on-accent': on, '--paper': t.paper,
    '--accent-soft': `color-mix(in oklab, ${acc} ${t.dark ? 20 : 14}%, transparent)`,
    '--danger': t.dark ? '#FF8A80' : '#B42318',
  }
  for (const [k, v] of Object.entries(vars)) target.style.setProperty(k, v)
  target.style.colorScheme = t.dark ? 'dark' : 'light'
  if (target === document.documentElement) {
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', t.chrome)
  }
}
