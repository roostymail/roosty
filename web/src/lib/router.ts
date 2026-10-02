import { createSignal } from 'solid-js'

// A tiny History API router: enough for a handful of top-level screens.
const [path, setPath] = createSignal(location.pathname)

window.addEventListener('popstate', () => setPath(location.pathname))

export const currentPath = path

export function navigate(to: string, replace = false) {
  if (to === location.pathname) return
  if (replace) history.replaceState(null, '', to)
  else history.pushState(null, '', to)
  setPath(to)
}
