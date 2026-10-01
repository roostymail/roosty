import { createSignal, Show } from 'solid-js'
import { api } from '../lib/api'
import { navigate } from '../lib/router'
import { brand } from '../lib/state'
import { Mark } from '../components/Logo'
import Icon from '../components/Icon'

export default function Login() {
  const [email, setEmail] = createSignal('')
  const [password, setPassword] = createSignal('')
  const [remember, setRemember] = createSignal(true)
  const [error, setError] = createSignal('')
  const [busy, setBusy] = createSignal(false)

  const submit = async (e: Event) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await api.post('/api/auth/login', { email: email(), password: password(), remember: remember() })
      location.href = '/'
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div class="login-page">
      <form class="login-card" onSubmit={submit}>
        <div class="login-logo">
          <Show when={brand()?.logos?.includes('light')} fallback={<span class="app-icon big"><Mark size={46} /></span>}>
            <img src="/branding/logo/light" alt={brand()?.name} class="login-custom-logo" />
          </Show>
        </div>
        <h1>{brand()?.loginTitle || 'Entrar no Roosty Mail'}</h1>
        <p class="muted">{brand()?.loginMessage || 'Use seu endereço de email completo e a senha da sua caixa.'}</p>
        <Show when={brand()?.setupNeeded}>
          <div class="banner warn">
            <Icon name="settings" size={16} />
            <span>Esta instância ainda não foi configurada. <a href="/admin/setup" onClick={(e) => { e.preventDefault(); navigate('/admin/setup') }}>Abrir configuração inicial</a></span>
          </div>
        </Show>
        <label class="field">
          <span>Email</span>
          <input id="login-email" type="email" autocomplete="username" placeholder="voce@seudominio.com" required value={email()} onInput={(e) => setEmail(e.currentTarget.value)} />
        </label>
        <label class="field">
          <span>Senha</span>
          <input id="login-password" type="password" autocomplete="current-password" required value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
        </label>
        <label class="check">
          <input id="login-remember" type="checkbox" checked={remember()} onChange={(e) => setRemember(e.currentTarget.checked)} />
          <span>Manter conectado por 30 dias</span>
        </label>
        <Show when={error()}>
          <div class="banner error" role="alert"><Icon name="alert" size={16} /><span>{error()}</span></div>
        </Show>
        <button class="btn primary block" type="submit" disabled={busy()}>{busy() ? 'Entrando…' : 'Entrar'}</button>
        <p class="login-note"><Icon name="lock" size={13} /> Sua senha só é usada para falar com o seu servidor de email.</p>
      </form>
      <footer class="login-footer">
        <Show when={brand()?.links?.support}><a href={brand()!.links.support} target="_blank" rel="noopener noreferrer">Ajuda</a></Show>
        <Show when={brand()?.links?.privacy}><a href={brand()!.links.privacy} target="_blank" rel="noopener noreferrer">Privacidade</a></Show>
        <span>Roosty Mail · open source</span>
      </footer>
    </div>
  )
}
