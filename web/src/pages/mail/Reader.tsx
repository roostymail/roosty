import DOMPurify from 'dompurify'
import { createEffect, For, onCleanup, Show } from 'solid-js'
import { batch } from '../../lib/api'
import { formatSize, fullDate, initials, toast, updatePrefs, me } from '../../lib/state'
import Icon from '../../components/Icon'
import { body, bodyLoading, closeMessage, moveTo, openMessage, selected, setFlag, view } from './store'
import { openComposer } from './Composer'

const FRAME_CSS = `
html,body{margin:0;padding:0;background:transparent}
body{padding:4px 2px;font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;color:#1a1a1a;overflow-wrap:anywhere}
img{max-width:100%;height:auto}
img[data-blocked]{display:inline-block;min-width:28px;min-height:20px;background:repeating-linear-gradient(45deg,#eee,#eee 6px,#f6f6f6 6px,#f6f6f6 12px);border-radius:4px}
table{max-width:100%}
pre.plain{white-space:pre-wrap;font:inherit;margin:0}
blockquote{margin:0 0 0 8px;padding-left:12px;border-left:3px solid #ddd;color:#555}
a{color:#2457c5}
`

function escapeHtml(s: string) {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!)
}

function textToHtml(t: string) {
  const esc = escapeHtml(t)
  const linked = esc.replace(/\bhttps?:\/\/[^\s<>"']+/g, (u) => `<a href="${u}" rel="noopener noreferrer">${u}</a>`)
  return `<pre class="plain">${linked}</pre>`
}

/** Email HTML is sanitized on the server and again here, then rendered in a
 *  sandboxed iframe without script permission and with its own CSP. */
function MailFrame(props: { html: string }) {
  let frame!: HTMLIFrameElement
  let ro: ResizeObserver | undefined
  const doc = () => {
    const clean = DOMPurify.sanitize(props.html, {
      FORBID_TAGS: ['style', 'svg', 'math', 'form', 'input', 'button', 'textarea', 'select', 'iframe', 'object', 'embed'],
      FORBID_ATTR: ['srcset'],
      ALLOW_DATA_ATTR: true,
    })
    return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'"><base target="_blank"><style>${FRAME_CSS}</style></head><body>${clean}</body></html>`
  }
  const fit = () => {
    const d = frame.contentDocument
    if (d?.documentElement) frame.style.height = `${d.documentElement.scrollHeight + 4}px`
  }
  createEffect(() => {
    frame.srcdoc = doc()
  })
  const onLoad = () => {
    fit()
    ro?.disconnect()
    const d = frame.contentDocument
    if (d?.body) {
      ro = new ResizeObserver(fit)
      ro.observe(d.body)
      d.querySelectorAll('img').forEach((img) => img.addEventListener('load', fit))
    }
  }
  onCleanup(() => ro?.disconnect())
  return <iframe ref={frame} class="mail-frame" title="Conteúdo do email" sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox" onLoad={onLoad} />
}

export default function Reader() {
  const m = selected
  const b = body
  const sender = () => b()?.message.from[0] || m()?.from[0]
  const trust = async () => {
    const email = sender()?.email
    if (!email) return
    try {
      await batch([['Sender/trust', { email }, 't']])
      const list = [...(me()?.prefs.trustedSenders || []), email.toLowerCase()]
      updatePrefs({ trustedSenders: list })
      toast(`Imagens de ${email} serão sempre exibidas`)
      openMessage(m()!, true)
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  const isRole = (r: string) => view().mailbox === r

  return (
    <section class="reader" aria-label="Leitura">
      <Show
        when={m()}
        fallback={
          <div class="empty">
            <Icon name="mailOpen" size={40} />
            <p class="empty-title">Selecione uma mensagem</p>
            <p class="muted">Escolha um email na lista para ler aqui.</p>
          </div>
        }
      >
        <div class="reader-bar">
          <button class="icon-btn mobile-only" aria-label="Voltar" onClick={closeMessage}><Icon name="arrowLeft" /></button>
          <div class="reader-actions desktop-only">
            <button class="pill" onClick={() => openComposer('reply')}><Icon name="reply" size={16} />Responder</button>
            <button class="pill" onClick={() => openComposer('replyAll')}><Icon name="replyAll" size={16} />Responder a todos</button>
            <button class="pill" onClick={() => openComposer('forward')}><Icon name="forward" size={16} />Encaminhar</button>
          </div>
          <span class="grow" />
          <button class="icon-btn" title="Arquivar (e)" aria-label="Arquivar" onClick={() => moveTo(m()!, 'archive', 'Mensagem arquivada')}><Icon name="archive" /></button>
          <button class="icon-btn" title="Spam (!)" aria-label="Marcar como spam" onClick={() => moveTo(m()!, 'junk', 'Movida para Spam')}><Icon name="alert" /></button>
          <button class="icon-btn danger" title="Excluir (#)" aria-label="Excluir" onClick={() => moveTo(m()!, 'destroy', 'Mensagem excluída')}><Icon name="trash" /></button>
          <button class="icon-btn" title="Marcar como não lida" aria-label="Marcar como não lida" onClick={() => { setFlag(m()!, 'seen', false); closeMessage() }}><Icon name="mail" /></button>
          <button class="icon-btn desktop-only" title="Fechar" aria-label="Fechar" onClick={closeMessage}><Icon name="x" /></button>
        </div>
        <div class="reader-scroll">
          <div class="reader-head">
            <h1 class="subject">
              {m()!.subject || '(sem assunto)'}
              <button class={`icon-btn star ${m()!.flagged ? 'on' : ''}`} aria-label="Favorito" onClick={() => setFlag(m()!, 'flagged', !m()!.flagged)}><Icon name="star" /></button>
            </h1>
            <div class="sender">
              <span class="avatar accent">{initials(sender()?.name || sender()?.email || '?')}</span>
              <div class="sender-info">
                <div><b>{sender()?.name || sender()?.email}</b> <Show when={sender()?.name}><span class="muted">&lt;{sender()?.email}&gt;</span></Show></div>
                <div class="muted small">para {(b()?.message.to || m()!.to).map((a) => a.name || a.email).join(', ') || 'mim'}
                  <Show when={b()?.message.cc?.length}> · cc {b()!.message.cc.map((a) => a.name || a.email).join(', ')}</Show>
                </div>
              </div>
              <span class="muted small date">{fullDate(m()!.date)}</span>
            </div>
          </div>
          <Show when={b() && (b()!.blocked > 0 || b()!.trackers > 0) && !b()!.remoteAllowed}>
            <div class="privacy">
              <Icon name="shield" size={18} class="accent-text" />
              <span class="grow">
                {b()!.blocked > 0 ? `${b()!.blocked} ${b()!.blocked === 1 ? 'imagem remota oculta' : 'imagens remotas ocultas'}` : ''}
                {b()!.blocked > 0 && b()!.trackers > 0 ? ' e ' : ''}
                {b()!.trackers > 0 ? `${b()!.trackers} ${b()!.trackers === 1 ? 'rastreador bloqueado' : 'rastreadores bloqueados'}` : ''} para proteger sua privacidade.
              </span>
              <Show when={b()!.blocked > 0}>
                <button class="chip-btn" onClick={() => openMessage(m()!, true)}>Mostrar imagens</button>
                <button class="chip-btn ghost desktop-only" onClick={trust}>Sempre deste remetente</button>
              </Show>
            </div>
          </Show>
          <Show when={b()} fallback={<div class="paper loading"><div class="skeleton" /><div class="skeleton short" /><div class="skeleton" /></div>}>
            <div class="paper" classList={{ dim: bodyLoading() }}>
              <MailFrame html={b()!.hasHtml ? b()!.html : textToHtml(b()!.message.text || '')} />
            </div>
            <Show when={b()!.message.attachments.length}>
              <div class="attachments">
                <For each={b()!.message.attachments}>
                  {(a) => (
                    <a class="att" href={`/api/attachment?m=${encodeURIComponent(m()!.mailbox)}&id=${m()!.id}&part=${a.index}`} download={a.name}>
                      <Icon name="file" size={16} /><span class="att-name">{a.name}</span><span class="muted small">{formatSize(a.size)}</span>
                    </a>
                  )}
                </For>
              </div>
            </Show>
          </Show>
          <Show when={!isRole('Drafts')}>
            <button class="quick-reply desktop-only" onClick={() => openComposer('reply')}>
              <Icon name="reply" /><span>Responder para {sender()?.name || sender()?.email}…</span>
            </button>
          </Show>
        </div>
        <div class="reader-bottom mobile-only">
          <button class="btn primary" onClick={() => openComposer('reply')}><Icon name="reply" size={16} />Responder</button>
          <button class="btn" onClick={() => openComposer('forward')}><Icon name="forward" size={16} />Encaminhar</button>
        </div>
      </Show>
    </section>
  )
}
