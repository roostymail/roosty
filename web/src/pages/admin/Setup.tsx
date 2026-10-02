import { createEffect, createResource, createSignal, For, Show } from 'solid-js'
import { api } from '../../lib/api'
import { applyTheme, THEMES } from '../../lib/theme'
import Icon from '../../components/Icon'
import { Brand } from '../../components/Logo'
import { EndpointFields, ListEditor, Settings, TestPanel, TestResult, ThemePicker } from './common'

const STEPS = ['Conta de administrador', 'Servidor de email', 'Domínios', 'Branding', 'Concluir']

export default function Setup() {
  const [status] = createResource(() => api.get<{ needed: boolean; settings: Settings; locked: Record<string, boolean> }>('/api/setup/status'))
  const [step, setStep] = createSignal(0)
  const [token, setToken] = createSignal(new URLSearchParams(location.search).get('token') || '')
  const [username, setUsername] = createSignal('admin')
  const [password, setPassword] = createSignal('')
  const [password2, setPassword2] = createSignal('')
  const [cfg, setCfg] = createSignal<Settings | null>(null)
  const [test, setTest] = createSignal<TestResult | null>(null)
  const [busy, setBusy] = createSignal(false)
  const [error, setError] = createSignal('')

  createEffect(() => {
    const st = status()
    if (st && !cfg()) {
      const x = structuredClone(st.settings)
      x.access.domains ||= []
      x.access.accounts ||= []
      x.branding.themes ||= THEMES.map((t) => t.id)
      setCfg(x)
    }
  })
  const s = cfg
  const locked = () => status()?.locked || {}
  const upd = (fn: (x: Settings) => void) => {
    const x = structuredClone(s()!)
    fn(x)
    setCfg(x)
  }

  const runTest = async () => {
    setBusy(true)
    setError('')
    try {
      setTest(await api.post<TestResult>('/api/setup/test', { token: token(), mail: s()!.mail }))
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const next = () => {
    setError('')
    if (step() === 0) {
      if (!token().trim()) return setError('Informe o código de configuração mostrado no log do container.')
      if (username().trim().length < 3) return setError('O usuário precisa ter pelo menos 3 caracteres.')
      if (password().length < 10) return setError('Use uma senha com pelo menos 10 caracteres.')
      if (password() !== password2()) return setError('As senhas não conferem.')
    }
    if (step() === 1 && (!s()!.mail.imap.host || !s()!.mail.smtp.host)) return setError('Preencha os servidores IMAP e SMTP.')
    setStep(step() + 1)
  }

  const finish = async () => {
    setBusy(true)
    setError('')
    try {
      await api.post('/api/setup/complete', { token: token(), username: username().trim(), password: password(), settings: s() })
      location.href = '/admin'
    } catch (e) {
      setError((e as Error).message)
      if ((e as Error).message.includes('código')) setStep(0)
    } finally {
      setBusy(false)
    }
  }

  applyTheme('bege')

  return (
    <div class="setup-page">
      <header class="setup-top"><Brand /><span class="badge">Configuração inicial</span></header>
      <Show when={status()} fallback={<div class="boot" />}>
        <Show when={status()!.needed} fallback={
          <div class="setup-card"><h1>Tudo pronto</h1><p class="muted">A configuração inicial já foi concluída.</p><a class="btn primary" href="/admin/login">Entrar no painel</a></div>
        }>
          <div class="setup-layout">
            <ol class="stepper">
              <For each={STEPS}>
                {(label, i) => (
                  <li classList={{ done: i() < step(), on: i() === step() }}>
                    <span class="step-num">{i() < step() ? <Icon name="check" size={14} /> : i() + 1}</span>{label}
                  </li>
                )}
              </For>
            </ol>
            <div class="setup-card">
              <Show when={step() === 0}>
                <h1>Conta de administrador</h1>
                <p class="muted">Esta conta acessa só o painel de administração. Ela é separada das caixas de email.</p>
                <label class="field"><span>Código de configuração</span>
                  <input id="setup-token" value={token()} onInput={(e) => setToken(e.currentTarget.value)} placeholder="Aparece no log do container" />
                  <small class="muted">Rode <code>docker compose logs roosty</code> e procure por “setup”.</small>
                </label>
                <label class="field"><span>Usuário</span><input id="setup-user" value={username()} onInput={(e) => setUsername(e.currentTarget.value)} autocomplete="username" /></label>
                <div class="grid2">
                  <label class="field"><span>Senha</span><input id="setup-pass" type="password" value={password()} onInput={(e) => setPassword(e.currentTarget.value)} autocomplete="new-password" /></label>
                  <label class="field"><span>Confirmar senha</span><input id="setup-pass2" type="password" value={password2()} onInput={(e) => setPassword2(e.currentTarget.value)} autocomplete="new-password" /></label>
                </div>
              </Show>
              <Show when={step() === 1 && s()}>
                <h1>Servidor de email</h1>
                <p class="muted">O Roosty conversa com o seu servidor por IMAP (leitura) e SMTP (envio). Nenhum email é guardado aqui.</p>
                <Show when={Object.keys(locked()).some((k) => k.startsWith('mail.'))}>
                  <div class="banner info"><Icon name="lock" size={16} /><span>Alguns campos vieram das variáveis de ambiente do container e não podem ser alterados aqui.</span></div>
                </Show>
                <EndpointFields label="IMAP (leitura)" prefix="mail.imap" value={s()!.mail.imap} locked={locked()} onChange={(v) => upd((x) => (x.mail.imap = v))} />
                <EndpointFields label="SMTP (envio)" prefix="mail.smtp" value={s()!.mail.smtp} locked={locked()} onChange={(v) => upd((x) => (x.mail.smtp = v))} />
                <button class="btn" disabled={busy()} onClick={runTest}><Icon name="server" size={16} />{busy() ? 'Testando…' : 'Testar conexão'}</button>
                <TestPanel result={test()} />
              </Show>
              <Show when={step() === 2 && s()}>
                <h1>Quem pode entrar</h1>
                <p class="muted">As senhas continuam no servidor de email. Aqui você define quais endereços podem usar este webmail.</p>
                <div class="mode-cards">
                  <button class="mode-card" classList={{ on: s()!.access.mode !== 'list' }} disabled={locked()['access.mode']} onClick={() => upd((x) => (x.access.mode = 'domains'))}>
                    <Icon name="globe" /><b>Por domínio</b><span class="muted small">Qualquer caixa válida dos domínios liberados.</span>
                  </button>
                  <button class="mode-card" classList={{ on: s()!.access.mode === 'list' }} disabled={locked()['access.mode']} onClick={() => upd((x) => (x.access.mode = 'list'))}>
                    <Icon name="users" /><b>Lista de contas</b><span class="muted small">Só os endereços que você cadastrar.</span>
                  </button>
                  <div class="mode-card soon"><Icon name="server" /><b>Criar caixas via API</b><span class="muted small">Em breve</span></div>
                </div>
                <Show when={s()!.access.mode !== 'list'} fallback={
                  <ListEditor items={s()!.access.accounts || []} placeholder="nome@dominio.com" disabled={locked()['access.accounts']} onChange={(l) => upd((x) => (x.access.accounts = l))} />
                }>
                  <ListEditor items={s()!.access.domains || []} placeholder="seudominio.com" disabled={locked()['access.domains']} onChange={(l) => upd((x) => (x.access.domains = l))} />
                  <p class="muted small">Deixe vazio para aceitar qualquer domínio que o servidor de email autenticar.</p>
                </Show>
              </Show>
              <Show when={step() === 3 && s()}>
                <h1>Branding</h1>
                <p class="muted">Dá para mudar tudo depois no painel, inclusive enviar logos.</p>
                <label class="field"><span>Nome da instância</span><input value={s()!.branding.name} disabled={locked()['branding.name']} onInput={(e) => upd((x) => (x.branding.name = e.currentTarget.value))} /></label>
                <label class="field"><span>Título da tela de login</span><input value={s()!.branding.loginTitle} onInput={(e) => upd((x) => (x.branding.loginTitle = e.currentTarget.value))} /></label>
                <label class="field"><span>Temas disponíveis e tema padrão</span></label>
                <ThemePicker value={s()!.branding.themes || []} def={s()!.branding.defaultTheme} onChange={(t, d) => upd((x) => { x.branding.themes = t; x.branding.defaultTheme = d })} />
                <label class="field"><span>Tema padrão</span>
                  <select value={s()!.branding.defaultTheme} onChange={(e) => upd((x) => (x.branding.defaultTheme = e.currentTarget.value))}>
                    <For each={THEMES.filter((t) => (s()!.branding.themes || []).includes(t.id))}>{(t) => <option value={t.id}>{t.name}</option>}</For>
                  </select>
                </label>
              </Show>
              <Show when={step() === 4 && s()}>
                <h1>Revisar e concluir</h1>
                <dl class="review">
                  <dt>Administrador</dt><dd>{username()}</dd>
                  <dt>IMAP</dt><dd>{s()!.mail.imap.host}:{s()!.mail.imap.port} ({s()!.mail.imap.security})</dd>
                  <dt>SMTP</dt><dd>{s()!.mail.smtp.host}:{s()!.mail.smtp.port} ({s()!.mail.smtp.security})</dd>
                  <dt>Acesso</dt><dd>{s()!.access.mode === 'list' ? `${(s()!.access.accounts || []).length} contas` : (s()!.access.domains || []).join(', ') || 'qualquer domínio'}</dd>
                  <dt>Nome</dt><dd>{s()!.branding.name}</dd>
                </dl>
              </Show>
              <Show when={error()}><div class="banner error" role="alert"><Icon name="alert" size={16} /><span>{error()}</span></div></Show>
              <div class="setup-foot">
                <Show when={step() > 0}><button class="btn" onClick={() => setStep(step() - 1)}>Voltar</button></Show>
                <span class="grow" />
                <Show when={step() < 4} fallback={<button class="btn primary" disabled={busy()} onClick={finish}>{busy() ? 'Concluindo…' : 'Concluir configuração'}</button>}>
                  <button class="btn primary" onClick={next}>Continuar</button>
                </Show>
              </div>
            </div>
          </div>
        </Show>
      </Show>
    </div>
  )
}
