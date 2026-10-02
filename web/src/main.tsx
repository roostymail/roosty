import '@fontsource-variable/figtree'
import './styles.css'
import { render } from 'solid-js/web'
import { createResource, For, Match, Show, Switch } from 'solid-js'
import { currentPath } from './lib/router'
import { brand, loadBranding, toasts, setToasts } from './lib/state'
import Login from './pages/Login'
import MailApp from './pages/Mail'
import Setup from './pages/admin/Setup'
import AdminLogin from './pages/admin/AdminLogin'
import Admin from './pages/admin/Admin'
import Icon from './components/Icon'

function Toasts() {
  return (
    <div class="toasts" role="status" aria-live="polite">
      <For each={toasts()}>
        {(t) => (
          <div class={`toast ${t.kind}`}>
            <span>{t.text}</span>
            <Show when={t.action}>
              <button class="link" onClick={() => { t.action!.run(); setToasts((l) => l.filter((x) => x.id !== t.id)) }}>{t.action!.label}</button>
            </Show>
            <button class="icon-btn sm" aria-label="Fechar" onClick={() => setToasts((l) => l.filter((x) => x.id !== t.id))}>
              <Icon name="x" size={14} />
            </button>
          </div>
        )}
      </For>
    </div>
  )
}

function App() {
  const [ready] = createResource(loadBranding)
  const p = currentPath
  return (
    <Show when={ready.state !== 'pending'} fallback={<div class="boot" />}>
      <Switch fallback={<MailApp />}>
        <Match when={p() === '/admin/setup' || (brand()?.setupNeeded && p().startsWith('/admin'))}><Setup /></Match>
        <Match when={p() === '/admin/login'}><AdminLogin /></Match>
        <Match when={p().startsWith('/admin')}><Admin /></Match>
        <Match when={p() === '/login'}><Login /></Match>
      </Switch>
      <Toasts />
    </Show>
  )
}

render(() => <App />, document.getElementById('app')!)
