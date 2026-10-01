import { Show } from 'solid-js'
import { brand } from '../lib/state'

// Origami pigeon mark from the Figma branding page.
const PATHS = [
  'M104.456 44.682L163.702 63.063L178.673 30.976L203.885 23.771L183.623 15.334L162.206 0.341003L135.212 4.499L114.191 21.307L104.456 44.682Z',
  'M102.608 49.17L79.629 105.556L115.005 184.635L171.226 115.929L162.866 67.87L102.608 49.17Z',
  'M77.264 112.101L55.473 173.514L98.076 242.506L111.848 189.409L77.264 112.101Z',
  'M116.611 190.322L101.816 247.357L162.778 279.873L170.995 123.849L116.611 190.322Z',
  'M178.585 120.813L278.784 323.345L371.03 333.586L229.493 138.435L178.585 120.813Z',
  'M175.736 125.961L167.508 282.15L272.327 321.2L175.736 125.961Z',
  'M283.987 328.79L398.31 439.098L374.242 338.822L283.987 328.79Z',
  'M143 275L154 277.2L140.8 334.4H132L143 275Z',
  'M165 281.6L176 283.8L171.6 332.2H162.8L165 281.6Z',
  'M48.4 338.8L272.8 328.9L277.2 342.1L48.4 344.3V338.8Z',
  'M92.4 326.7L89.76 297.55L69.19 276.87L71.83 305.91L92.4 326.7Z',
  'M66 328.9L47.96 305.91L19.36 299.75L37.4 322.74L66 328.9Z',
  'M44 339.9L22 331.65L0 339.9L22 348.15L44 339.9Z',
]

export function Mark(props: { size?: number }) {
  const s = () => props.size ?? 28
  return (
    <svg width={s() * 0.9} height={s()} viewBox="0 0 399 440" fill="currentColor" aria-hidden="true">
      {PATHS.map((d) => <path d={d} />)}
    </svg>
  )
}

/** App icon: olive tile with the mark, or the instance's uploaded icon. */
export function AppIcon(props: { size?: number }) {
  const s = () => props.size ?? 32
  return (
    <Show
      when={!brand()?.logos?.includes('icon')}
      fallback={<img src="/branding/logo/icon" width={s()} height={s()} alt="" style={{ 'border-radius': `${s() * 0.23}px` }} />}
    >
      <span class="app-icon" style={{ width: `${s()}px`, height: `${s()}px`, 'border-radius': `${s() * 0.23}px` }}>
        <Mark size={s() * 0.7} />
      </span>
    </Show>
  )
}

export function Brand(props: { size?: number }) {
  const name = () => brand()?.name || 'Roosty Mail'
  const parts = () => {
    const n = name()
    const i = n.lastIndexOf(' ')
    return i > 0 ? [n.slice(0, i), n.slice(i)] : [n, '']
  }
  return (
    <span class="brand">
      <AppIcon size={props.size ?? 30} />
      <span class="brand-name">
        <b>{parts()[0]}</b>
        {parts()[1]}
      </span>
    </span>
  )
}
