import { createResource, createSignal, For, Show } from 'solid-js'
import { api } from '../../lib/api'
import { navigate } from '../../lib/router'
import { brand, loadBranding, shortDate, toast } from '../../lib/state'
import { applyTheme, contrast, themeById, THEMES, validHex } from '../../lib/theme'
import Icon from '../../components/Icon'
import { Brand, Mark } from '../../components/Logo'
import { EndpointFields, ListEditor, Locked, Settings, TestPanel, TestResult, ThemePicker } from './common'

type Section = 'overview' | 'access' | 'branding' | 'server'

interface Account { email: string; firstSeen: number; lastLogin: number; blocked: boolean; sessions: number }

export default function Admin() {
  const [section, setSection] = createSignal<Section>((location.pathname.split('/')[2] as Section) || 'overview')
  const [me] = createResource(async () => {
    try {
      return await api.get<{ username: string }>('/api/admin/me')
    } catch {
      navigate('/admin/login', true)
      return null
    }
  })
  const [data, { mutate }] = createResource(me, () => api.get<{ settings: Settings; locked: Record<string, boolean> }>('/api/admin/settings'))
  const [accounts, { refetch: refetchAccounts }] = createResource(me, () => api.get<{ list: Account[] }>('/api/admin/accounts'))
  const [overview] = createResource(me, () => api.get<{ accounts: number; sessions: number; version: string; configured: boolean }>('/api/admin/overview'))
  const [draft, setDraft] = createSignal<Settings | null>(null)
  const [saving, setSaving] = createSignal(false)
  const [test, setTest] = createSignal<TestResult | null>(null)

  applyTheme('bege')

  const s = () => draft() || data()?.settings || null
  const locked = () => data()?.locked || {}
  const upd = (fn: (x: Settings) => void) => {
    const x = structuredClone(s()!)
    x.access.domains ||= []
    x.access.accounts ||= []
    x.branding.themes ||= THEMES.map((t) => t.id)
    fn(x)
    setDraft(x)
  }
  const go = (sec: Section) => {
    setSection(sec)
    history.replaceState(null, '', sec === 'overview' ? '/admin' : `/admin/${sec}`)
  }
  const save = async () => {
    setSaving(true)
    try {
      const r = await api.put<{ settings: Settings; locked: Record<string, boolean> }>('/api/admin/settings', s())
      mutate(r)
      setDraft(null)
      await loadBranding()
      applyTheme('bege')
      toast('Configurações salvas')
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setSaving(false)
    }
  }
  const act = async (action: string, email: string) => {
    try {
      await api.post(`/api/admin/accounts/${action}`, { email })
      refetchAccounts()
      toast(action === 'block' ? `${email} bloqueado` : action === 'unblock' ? `${email} desbloqueado` : 'Sessões encerradas')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  const uploadLogo = async (kind: string, file?: File) => {
    if (!file) return
    const fd = new FormData()
    fd.append('file', file)
    try {
      await api.post(`/api/admin/logo/${kind}`, fd)
      await loadBranding()
      applyTheme('bege')
      toast('Logo atualizada')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  const removeLogo = async (kind: string) => {
    await api.del(`/api/admin/logo/${kind}`).catch(() => {})
    await loadBranding()
    applyTheme('bege')
  }
  const logout = async () => {
    await api.post('/api/admin/logout').catch(() => {})
    location.href = '/admin/login'
  }
  const nav: [Section, string, string][] = [['overview', 'Visão geral', 'home'], ['access', 'Domínios e contas', 'users'], ['branding', 'Branding', 'palette'], ['server', 'Servidor de email', 'server']]
  const previewTheme = () => themeById(s()?.branding.defaultTheme || 'claro')
  const previewAccent = () => (validHex(s()?.branding.accent || '') ? s()!.branding.accent : previewTheme().accent)

  return (
    <Show when={me() && s()} fallback={<div class="boot" />}>
      <div class="admin">
        <header class="topbar">
          <div class="topbar-brand"><Brand /><span class="badge">Admin</span></div>
          <span class="grow" />
          <a class="btn sm" href="/" target="_blank">Abrir webmail</a>
          <span class="muted small desktop-only">{me()!.username}</span>
          <button class="icon-btn" aria-label="Sair" title="Sair" onClick={logout}><Icon name="logout" /></button>
        </header>
        <div class="admin-body">
          <nav class="admin-nav">
            <For each={nav}>{([id, label, icon]) => <button class="nav-item" classList={{ active: section() === id }} onClick={() => go(id)}><Icon name={icon} />{label}</button>}</For>
          </nav>
          <main class="admin-main">
            <Show when={section() === 'overview'}>
              <h1>Visão geral</h1>
              <div class="stats">
                <div class="stat"><span class="muted small">Contas que já entraram</span><b>{overview()?.accounts ?? '…'}</b></div>
                <div class="stat"><span class="muted small">Sessões ativas</span><b>{overview()?.sessions ?? '…'}</b></div>
                <div class="stat"><span class="muted small">Servidor de email</span><b class="state">{overview()?.configured ? 'Configurado' : 'Pendente'}</b></div>
                <div class="stat"><span class="muted small">Versão</span><b>{overview()?.version}</b></div>
              </div>
              <div class="card">
                <h3>Servidor conectado</h3>
                <p class="muted">IMAP {s()!.mail.imap.host}:{s()!.mail.imap.port} · SMTP {s()!.mail.smtp.host}:{s()!.mail.smtp.port}</p>
                <button class="btn" onClick={() => go('server')}>Ver detalhes</button>
              </div>
            </Show>

            <Show when={section() === 'access'}>
              <h1>Domínios e contas</h1>
              <div class="card">
                <h3>Modo de acesso <Locked on={locked()['access.mode']} /></h3>
                <div class="mode-cards">
                  <button class="mode-card" classList={{ on: s()!.access.mode !== 'list' }} disabled={locked()['access.mode']} onClick={() => upd((x) => (x.access.mode = 'domains'))}>
                    <Icon name="globe" /><b>Por domínio</b><span class="muted small">Qualquer caixa válida dos domínios liberados.</span>
                  </button>
                  <button class="mode-card" classList={{ on: s()!.access.mode === 'list' }} disabled={locked()['access.mode']} onClick={() => upd((x) => (x.access.mode = 'list'))}>
                    <Icon name="users" /><b>Lista de contas</b><span class="muted small">Só os endereços cadastrados.</span>
                  </button>
                  <div class="mode-card soon"><Icon name="server" /><b>Criar caixas via API</b><span class="muted small">Em breve</span></div>
                </div>
                <Show when={s()!.access.mode !== 'list'} fallback={<>
                  <h3>Contas permitidas <Locked on={locked()['access.accounts']} /></h3>
                  <ListEditor items={s()!.access.accounts || []} placeholder="nome@dominio.com" disabled={locked()['access.accounts']} onChange={(l) => upd((x) => (x.access.accounts = l))} />
                </>}>
                  <h3>Domínios permitidos <Locked on={locked()['access.domains']} /></h3>
                  <ListEditor items={s()!.access.domains || []} placeholder="seudominio.com" disabled={locked()['access.domains']} onChange={(l) => upd((x) => (x.access.domains = l))} />
                </Show>
                <div class="card-foot"><button class="btn primary" disabled={!draft() || saving()} onClick={save}>{saving() ? 'Salvando…' : 'Salvar'}</button></div>
              </div>
              <div class="card">
                <h3>Contas que já entraram</h3>
                <div class="table-wrap">
                  <table class="table">
                    <thead><tr><th>Endereço</th><th>Último acesso</th><th>Sessões</th><th>Status</th><th /></tr></thead>
                    <tbody>
                      <For each={accounts()?.list || []} fallback={<tr><td colspan="5" class="muted">Ninguém entrou ainda.</td></tr>}>
                        {(a) => (
                          <tr>
                            <td><b>{a.email}</b></td>
                            <td>{a.lastLogin ? shortDate(new Date(a.lastLogin * 1000).toISOString()) : '—'}</td>
                            <td>{a.sessions}</td>
                            <td><span class="status" classList={{ bad: a.blocked }}>{a.blocked ? 'Bloqueada' : 'Ativa'}</span></td>
                            <td class="actions">
                              <button class="btn sm" disabled={!a.sessions} onClick={() => act('revoke', a.email)}>Encerrar sessões</button>
                              <button class="btn sm" classList={{ danger: !a.blocked }} onClick={() => act(a.blocked ? 'unblock' : 'block', a.email)}>{a.blocked ? 'Desbloquear' : 'Bloquear'}</button>
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </div>
            </Show>

            <Show when={section() === 'branding'}>
              <h1>Branding</h1>
              <Show when={Object.keys(locked()).some((k) => k.startsWith('branding.'))}>
                <div class="banner info"><Icon name="lock" size={16} /><span>Campos com cadeado vêm das variáveis de ambiente e só mudam no container.</span></div>
              </Show>
              <div class="branding-layout">
                <div class="card">
                  <label class="field"><span>Nome da instância <Locked on={locked()['branding.name']} /></span>
                    <input value={s()!.branding.name} disabled={locked()['branding.name']} onInput={(e) => upd((x) => (x.branding.name = e.currentTarget.value))} />
                  </label>
                  <h3>Logos</h3>
                  <div class="logo-slots">
                    <For each={[['light', 'Logo (tela de login)'], ['icon', 'Ícone quadrado']] as const}>
                      {([kind, label]) => (
                        <div class="logo-slot">
                          <Show when={brand()?.logos?.includes(kind)} fallback={<span class="logo-empty"><Icon name="upload" /></span>}>
                            <img src={`/branding/logo/${kind}?v=${Date.now()}`} alt="" />
                          </Show>
                          <div><b class="small">{label}</b><span class="muted small">PNG, JPG, WebP ou SVG até 1 MB</span>
                            <div class="row-inline">
                              <label class="btn sm">Enviar<input type="file" hidden accept="image/png,image/jpeg,image/webp,image/svg+xml" onChange={(e) => uploadLogo(kind, e.currentTarget.files?.[0])} /></label>
                              <Show when={brand()?.logos?.includes(kind)}><button class="btn sm" onClick={() => removeLogo(kind)}>Remover</button></Show>
                            </div>
                          </div>
                        </div>
                      )}
                    </For>
                  </div>
                  <label class="field"><span>Cor de destaque padrão <Locked on={locked()['branding.accent']} /></span>
                    <div class="row-inline">
                      <input type="color" value={previewAccent()} disabled={locked()['branding.accent']} onInput={(e) => upd((x) => (x.branding.accent = e.currentTarget.value))} />
                      <input value={s()!.branding.accent} placeholder="Usar a cor de cada tema" disabled={locked()['branding.accent']} onInput={(e) => upd((x) => (x.branding.accent = e.currentTarget.value.trim()))} />
                      <button class="btn sm" disabled={locked()['branding.accent']} onClick={() => upd((x) => (x.branding.accent = ''))}>Limpar</button>
                    </div>
                    <small class="muted">Contraste com o tema padrão: {contrast(previewAccent(), previewTheme().list).toFixed(1)}:1</small>
                  </label>
                  <h3>Temas disponíveis</h3>
                  <ThemePicker value={s()!.branding.themes || THEMES.map((t) => t.id)} def={s()!.branding.defaultTheme} onChange={(t, d) => upd((x) => { x.branding.themes = t; x.branding.defaultTheme = d })} />
                  <label class="field"><span>Tema padrão <Locked on={locked()['branding.defaultTheme']} /></span>
                    <select value={s()!.branding.defaultTheme} disabled={locked()['branding.defaultTheme']} onChange={(e) => upd((x) => (x.branding.defaultTheme = e.currentTarget.value))}>
                      <For each={THEMES.filter((t) => (s()!.branding.themes || THEMES.map((x) => x.id)).includes(t.id))}>{(t) => <option value={t.id}>{t.name}</option>}</For>
                    </select>
                  </label>
                  <h3>Tela de login</h3>
                  <label class="field"><span>Título</span><input value={s()!.branding.loginTitle} onInput={(e) => upd((x) => (x.branding.loginTitle = e.currentTarget.value))} /></label>
                  <label class="field"><span>Mensagem</span><textarea rows={2} value={s()!.branding.loginMessage} onInput={(e) => upd((x) => (x.branding.loginMessage = e.currentTarget.value))} /></label>
                  <div class="grid2">
                    <label class="field"><span>Link de suporte</span><input value={s()!.branding.links.support} placeholder="https://" onInput={(e) => upd((x) => (x.branding.links.support = e.currentTarget.value.trim()))} /></label>
                    <label class="field"><span>Link de privacidade</span><input value={s()!.branding.links.privacy} placeholder="https://" onInput={(e) => upd((x) => (x.branding.links.privacy = e.currentTarget.value.trim()))} /></label>
                  </div>
                  <details class="advanced">
                    <summary>CSS da instância (avançado)</summary>
                    <textarea class="mono" rows={6} value={s()!.branding.customCss} placeholder=".login-card { border-radius: 24px }" onInput={(e) => upd((x) => (x.branding.customCss = e.currentTarget.value))} />
                    <small class="muted">Aplicado para todos os usuários. Pode quebrar em atualizações de versão.</small>
                  </details>
                  <div class="card-foot"><button class="btn primary" disabled={!draft() || saving()} onClick={save}>{saving() ? 'Salvando…' : 'Salvar e publicar'}</button></div>
                </div>
                <aside class="preview" style={{ '--p-bg': previewTheme().chrome, '--p-list': previewTheme().list, '--p-text': previewTheme().text, '--p-muted': previewTheme().muted, '--p-border': previewTheme().border, '--p-acc': previewAccent(), '--p-on': contrast(previewAccent(), '#fff') >= contrast(previewAccent(), '#111') ? '#fff' : '#111' } as any}>
                  <span class="muted small">Prévia ao vivo</span>
                  <div class="p-login">
                    <Show when={brand()?.logos?.includes('light')} fallback={<span class="p-icon"><Mark size={22} /></span>}><img src={`/branding/logo/light`} alt="" class="p-logo" /></Show>
                    <b>{s()!.branding.loginTitle || 'Entrar'}</b>
                    <span class="p-muted">{s()!.branding.loginMessage}</span>
                    <span class="p-input" /><span class="p-input" />
                    <span class="p-btn">Entrar</span>
                  </div>
                  <div class="p-app">
                    <div class="p-top"><span class="p-icon sm"><Mark size={11} /></span><b>{s()!.branding.name}</b></div>
                    <div class="p-cols">
                      <div class="p-side"><span class="p-btn sm">Escrever</span><i class="on" /><i /><i /></div>
                      <div class="p-list"><i class="on" /><i /><i /><i /></div>
                    </div>
                  </div>
                </aside>
              </div>
            </Show>

            <Show when={section() === 'server'}>
              <h1>Servidor de email</h1>
              <div class="card">
                <EndpointFields label="IMAP (leitura)" prefix="mail.imap" value={s()!.mail.imap} locked={locked()} onChange={(v) => upd((x) => (x.mail.imap = v))} />
                <EndpointFields label="SMTP (envio)" prefix="mail.smtp" value={s()!.mail.smtp} locked={locked()} onChange={(v) => upd((x) => (x.mail.smtp = v))} />
                <div class="card-foot">
                  <button class="btn" onClick={async () => { try { setTest(await api.post<TestResult>('/api/admin/test', s()!.mail)) } catch (e) { toast((e as Error).message, 'error') } }}><Icon name="server" size={16} />Testar conexão</button>
                  <span class="grow" />
                  <button class="btn primary" disabled={!draft() || saving()} onClick={save}>Salvar</button>
                </div>
                <TestPanel result={test()} />
              </div>
            </Show>
          </main>
        </div>
      </div>
    </Show>
  )
}
