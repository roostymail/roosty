import { For, JSX, Show } from 'solid-js'
import Icon from '../../components/Icon'
import { THEMES } from '../../lib/theme'

export interface Endpoint { host: string; port: number; security: string }
export interface MailServer { imap: Endpoint; smtp: Endpoint; skipTlsVerify: boolean }
export interface Settings {
  mail: MailServer
  access: { mode: string; domains: string[] | null; accounts: string[] | null }
  branding: {
    name: string; accent: string; defaultTheme: string; themes: string[] | null; loginTitle: string; loginMessage: string
    links: { support: string; privacy: string }; customCss: string; logos: string[] | null
  }
}
export interface TestResult { imapOk: boolean; imapError?: string; imapCaps: string[]; smtpOk: boolean; smtpError?: string; smtpAuth: boolean }

export function Locked(props: { on?: boolean }) {
  return <Show when={props.on}><span class="locked" title="Definido por variável de ambiente"><Icon name="lock" size={13} /></span></Show>
}

export function EndpointFields(props: { label: string; value: Endpoint; prefix: string; locked: Record<string, boolean>; onChange: (e: Endpoint) => void }) {
  const l = (k: string) => !!props.locked[`${props.prefix}.${k}`]
  return (
    <fieldset class="endpoint">
      <legend>{props.label}</legend>
      <label class="field grow2"><span>Servidor <Locked on={l('host')} /></span>
        <input value={props.value.host} disabled={l('host')} placeholder="mail.seudominio.com" onInput={(e) => props.onChange({ ...props.value, host: e.currentTarget.value.trim() })} />
      </label>
      <label class="field"><span>Porta <Locked on={l('port')} /></span>
        <input type="number" value={props.value.port} disabled={l('port')} onInput={(e) => props.onChange({ ...props.value, port: Number(e.currentTarget.value) })} />
      </label>
      <label class="field"><span>Segurança <Locked on={l('security')} /></span>
        <select value={props.value.security} disabled={l('security')} onChange={(e) => props.onChange({ ...props.value, security: e.currentTarget.value })}>
          <option value="tls">SSL/TLS</option><option value="starttls">STARTTLS</option><option value="none">Nenhuma (só testes)</option>
        </select>
      </label>
    </fieldset>
  )
}

export function TestPanel(props: { result: TestResult | null }) {
  const important = ['IMAP4rev2', 'CONDSTORE', 'QRESYNC', 'IDLE', 'MOVE', 'SPECIAL-USE', 'SORT', 'UIDPLUS']
  return (
    <Show when={props.result}>
      {(r) => (
        <div class="test-result">
          <div class="test-line" classList={{ ok: r().imapOk, bad: !r().imapOk }}>
            <Icon name={r().imapOk ? 'check' : 'x'} size={16} /><b>IMAP</b>
            <span>{r().imapOk ? 'Conexão estabelecida' : r().imapError}</span>
          </div>
          <Show when={r().imapOk}>
            <div class="caps">
              <For each={important}>{(c) => <span class="cap" classList={{ on: r().imapCaps.includes(c) }}>{c}</span>}</For>
            </div>
          </Show>
          <div class="test-line" classList={{ ok: r().smtpOk, bad: !r().smtpOk }}>
            <Icon name={r().smtpOk ? 'check' : 'x'} size={16} /><b>SMTP</b>
            <span>{r().smtpOk ? (r().smtpAuth ? 'Conexão estabelecida, autenticação disponível' : 'Conectado, mas sem AUTH anunciado') : r().smtpError}</span>
          </div>
        </div>
      )}
    </Show>
  )
}

export function ListEditor(props: { items: string[]; placeholder: string; disabled?: boolean; onChange: (l: string[]) => void }) {
  let input!: HTMLInputElement
  const add = () => {
    const v = input.value.trim().toLowerCase()
    if (v && !props.items.includes(v)) props.onChange([...props.items, v])
    input.value = ''
  }
  return (
    <div class="list-editor">
      <div class="chips">
        <For each={props.items} fallback={<span class="muted small">Nenhum item</span>}>
          {(it) => (
            <span class="chip">{it}<Show when={!props.disabled}><button aria-label={`Remover ${it}`} onClick={() => props.onChange(props.items.filter((x) => x !== it))}><Icon name="x" size={12} /></button></Show></span>
          )}
        </For>
      </div>
      <Show when={!props.disabled}>
        <div class="row-inline">
          <input ref={input} placeholder={props.placeholder} onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); add() } }} />
          <button class="btn" type="button" onClick={add}>Adicionar</button>
        </div>
      </Show>
    </div>
  )
}

export function ThemePicker(props: { value: string[]; def: string; onChange: (themes: string[], def: string) => void }): JSX.Element {
  return (
    <div class="theme-checks">
      <For each={THEMES}>
        {(t) => (
          <label class="theme-check">
            <input type="checkbox" checked={props.value.includes(t.id)} onChange={(e) => {
              const next = e.currentTarget.checked ? [...props.value, t.id] : props.value.filter((x) => x !== t.id)
              props.onChange(next, next.includes(props.def) ? props.def : next[0] || 'claro')
            }} />
            <span class="tc-swatch" style={{ background: t.list, 'border-color': t.border }}><i style={{ background: t.accent }} /></span>
            <span>{t.name}</span>
            <Show when={props.def === t.id}><span class="badge">padrão</span></Show>
          </label>
        )}
      </For>
    </div>
  )
}
