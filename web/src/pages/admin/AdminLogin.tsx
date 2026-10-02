import { createSignal, Show } from 'solid-js'
import { api } from '../../lib/api'
import { applyTheme } from '../../lib/theme'
import Icon from '../../components/Icon'
import { Brand } from '../../components/Logo'

export default function AdminLogin() {
  const [user, setUser] = createSignal('')
  const [pass, setPass] = createSignal('')
  const [error, setError] = createSignal('')
  const [busy, setBusy] = createSignal(false)
  applyTheme('bege')
  const submit = async (e: Event) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.post('/api/admin/login', { username: user(), password: pass() })
      location.href = '/admin'
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div class="login-page">
      <form class="login-card" onSubmit={submit}>
        <div class="login-logo"><Brand size={36} /></div>
        <h1>Painel de administração</h1>
        <p class="muted">Entre com a conta de administrador criada na configuração inicial.</p>
        <label class="field"><span>Usuário</span><input id="admin-user" autocomplete="username" value={user()} onInput={(e) => setUser(e.currentTarget.value)} required /></label>
        <label class="field"><span>Senha</span><input id="admin-pass" type="password" autocomplete="current-password" value={pass()} onInput={(e) => setPass(e.currentTarget.value)} required /></label>
        <Show when={error()}><div class="banner error" role="alert"><Icon name="alert" size={16} /><span>{error()}</span></div></Show>
        <button class="btn primary block" disabled={busy()}>{busy() ? 'Entrando…' : 'Entrar'}</button>
        <p class="login-note"><a href="/login">Ir para o webmail</a></p>
      </form>
    </div>
  )
}
