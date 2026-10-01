import DOMPurify from 'dompurify'
import { createSignal, For, onMount, Show } from 'solid-js'
import { batch, upload, Upload } from '../../lib/api'
import { formatSize, fullDate, me, toast } from '../../lib/state'
import Icon from '../../components/Icon'
import { body, loadMailboxes, loadMessages, selected, view, roleBox } from './store'

interface Draft {
  key: number
  to: string; cc: string; bcc: string; subject: string; html: string
  inReplyTo?: string; references?: string; showCc: boolean
}

export const [composer, setComposer] = createSignal<Draft | null>(null)
const [minimized, setMinimized] = createSignal(false)
let keySeq = 0

const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)
const fmt = (a: { name?: string; email: string }) => (a.name ? `${a.name} <${a.email}>` : a.email)

function signature() {
  const s = me()?.prefs.signature
  return s ? `<p><br></p><div class="signature">-- <br>${esc(s).replace(/\n/g, '<br>')}</div>` : ''
}

export function openComposer(mode: 'new' | 'reply' | 'replyAll' | 'forward' = 'new') {
  if (composer() && mode === 'new') {
    setMinimized(false)
    return
  }
  const d: Draft = { key: ++keySeq, to: '', cc: '', bcc: '', subject: '', html: `<p><br></p>${signature()}`, showCc: false }
  const m = selected()
  const b = body()
  if (mode !== 'new' && m && b) {
    const msg = b.message
    const quoted = DOMPurify.sanitize(b.hasHtml ? b.html : esc(msg.text || '').replace(/\n/g, '<br>'), { FORBID_TAGS: ['style', 'img'] })
    const who = msg.from[0] ? fmt(msg.from[0]) : ''
    if (mode === 'forward') {
      d.subject = /^(fwd?|enc):/i.test(msg.subject) ? msg.subject : `Fwd: ${msg.subject}`
      d.html = `<p><br></p>${signature()}<p>---------- Mensagem encaminhada ----------<br>De: ${esc(who)}<br>Data: ${esc(fullDate(msg.date))}<br>Assunto: ${esc(msg.subject)}<br>Para: ${esc(msg.to.map(fmt).join(', '))}</p>${quoted}`
    } else {
      const self = me()?.email.toLowerCase()
      const replyTo = msg.replyTo?.length ? msg.replyTo : msg.from
      d.to = replyTo.map(fmt).join(', ')
      if (mode === 'replyAll') {
        const others = [...msg.to, ...(msg.cc || [])].filter((a) => a.email.toLowerCase() !== self && !replyTo.some((r) => r.email === a.email))
        if (others.length) {
          d.cc = others.map(fmt).join(', ')
          d.showCc = true
        }
      }
      d.subject = /^re:/i.test(msg.subject) ? msg.subject : `Re: ${msg.subject}`
      d.inReplyTo = msg.messageId
      d.references = msg.references
      d.html = `<p><br></p>${signature()}<p>Em ${esc(fullDate(msg.date))}, ${esc(who)} escreveu:</p><blockquote>${quoted}</blockquote>`
    }
  }
  setMinimized(false)
  setComposer(d)
}

const split = (s: string) => s.split(/[,;]\s*(?=(?:[^"]*"[^"]*")*[^"]*$)/).map((x) => x.trim()).filter(Boolean)

export default function Composer() {
  const d = composer()!
  const [to, setTo] = createSignal(d.to)
  const [cc, setCc] = createSignal(d.cc)
  const [bcc, setBcc] = createSignal(d.bcc)
  const [subject, setSubject] = createSignal(d.subject)
  const [showCc, setShowCc] = createSignal(d.showCc)
  const [files, setFiles] = createSignal<(Upload & { pending?: boolean })[]>([])
  const [sending, setSending] = createSignal(false)
  const [maximized, setMaximized] = createSignal(false)
  let editor!: HTMLDivElement
  let fileInput!: HTMLInputElement

  onMount(() => {
    editor.innerHTML = d.html
    if (d.to) {
      editor.focus()
      const r = document.createRange()
      r.setStart(editor, 0)
      r.collapse(true)
      getSelection()?.removeAllRanges()
      getSelection()?.addRange(r)
    }
  })

  const exec = (cmd: string, arg?: string) => {
    editor.focus()
    document.execCommand(cmd, false, arg)
  }
  const addLink = () => {
    const url = window.prompt('Endereço do link (https://…)')
    if (url && /^https?:\/\//i.test(url)) exec('createLink', url)
  }

  const addFiles = async (list: FileList | null) => {
    for (const f of Array.from(list || [])) {
      if (f.size > 25 * 1024 * 1024) {
        toast(`${f.name} é maior que 25 MB`, 'error')
        continue
      }
      const temp = { id: `tmp-${Math.random()}`, name: f.name, type: f.type, size: f.size, pending: true }
      setFiles((l) => [...l, temp])
      try {
        const u = await upload(f)
        setFiles((l) => l.map((x) => (x.id === temp.id ? u : x)))
      } catch (e) {
        setFiles((l) => l.filter((x) => x.id !== temp.id))
        toast((e as Error).message, 'error')
      }
    }
  }

  const payload = () => ({
    fromName: me()?.prefs.displayName || '',
    to: split(to()), cc: split(cc()), bcc: split(bcc()), subject: subject(),
    html: editor.innerHTML, inReplyTo: d.inReplyTo || '', references: d.references || '',
    attachments: files().filter((f) => !f.pending).map((f) => f.id),
  })

  const send = async () => {
    if (!split(to()).length && !split(cc()).length && !split(bcc()).length) {
      toast('Adicione pelo menos um destinatário', 'error')
      return
    }
    if (files().some((f) => f.pending)) {
      toast('Aguarde o envio dos anexos', 'error')
      return
    }
    setSending(true)
    try {
      await batch([['Email/send', payload(), 's']])
      setComposer(null)
      toast('Mensagem enviada')
      loadMailboxes()
      if (view().mailbox === roleBox('sent')?.id) loadMessages()
    } catch (e) {
      toast((e as Error).message, 'error')
    } finally {
      setSending(false)
    }
  }

  const saveDraft = async () => {
    try {
      await batch([['Email/saveDraft', payload(), 'd']])
      setComposer(null)
      toast('Rascunho salvo')
      loadMailboxes()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const onKey = (e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      send()
    }
    if (e.key === 'Escape') setMinimized(true)
  }

  return (
    <div class="composer" classList={{ min: minimized(), max: maximized() }} role="dialog" aria-label="Escrever mensagem" onKeyDown={onKey}>
      <div class="composer-head" onClick={() => minimized() && setMinimized(false)}>
        <b class="grow">{subject() || 'Nova mensagem'}</b>
        <button class="icon-btn sm" aria-label="Minimizar" onClick={(e) => { e.stopPropagation(); setMinimized(!minimized()) }}><Icon name="minimize" size={16} /></button>
        <button class="icon-btn sm desktop-only" aria-label="Expandir" onClick={(e) => { e.stopPropagation(); setMaximized(!maximized()) }}><Icon name="maximize" size={16} /></button>
        <button class="icon-btn sm" aria-label="Fechar e salvar rascunho" onClick={(e) => { e.stopPropagation(); saveDraft() }}><Icon name="x" size={16} /></button>
      </div>
      <Show when={!minimized()}>
        <div class="composer-fields">
          <div class="cf"><span>De</span><div class="cf-static">{me()?.prefs.displayName ? `${me()!.prefs.displayName} <${me()!.email}>` : me()?.email}</div></div>
          <div class="cf">
            <span>Para</span>
            <input id="compose-to" value={to()} onInput={(e) => setTo(e.currentTarget.value)} placeholder="nome@exemplo.com" autocomplete="email" />
            <Show when={!showCc()}><button class="link small" onClick={() => setShowCc(true)}>Cc/Cco</button></Show>
          </div>
          <Show when={showCc()}>
            <div class="cf"><span>Cc</span><input id="compose-cc" value={cc()} onInput={(e) => setCc(e.currentTarget.value)} /></div>
            <div class="cf"><span>Cco</span><input id="compose-bcc" value={bcc()} onInput={(e) => setBcc(e.currentTarget.value)} /></div>
          </Show>
          <div class="cf"><span>Assunto</span><input id="compose-subject" value={subject()} onInput={(e) => setSubject(e.currentTarget.value)} /></div>
        </div>
        <div class="composer-tools">
          <button class="icon-btn sm" title="Negrito" aria-label="Negrito" onMouseDown={(e) => { e.preventDefault(); exec('bold') }}><Icon name="bold" size={16} /></button>
          <button class="icon-btn sm" title="Itálico" aria-label="Itálico" onMouseDown={(e) => { e.preventDefault(); exec('italic') }}><Icon name="italic" size={16} /></button>
          <button class="icon-btn sm" title="Lista" aria-label="Lista" onMouseDown={(e) => { e.preventDefault(); exec('insertUnorderedList') }}><Icon name="list" size={16} /></button>
          <button class="icon-btn sm" title="Link" aria-label="Link" onMouseDown={(e) => { e.preventDefault(); addLink() }}><Icon name="link" size={16} /></button>
          <button class="icon-btn sm" title="Anexar" aria-label="Anexar arquivo" onClick={() => fileInput.click()}><Icon name="paperclip" size={16} /></button>
          <input ref={fileInput} type="file" multiple hidden onChange={(e) => { addFiles(e.currentTarget.files); e.currentTarget.value = '' }} />
        </div>
        <div
          ref={editor}
          class="composer-editor"
          contentEditable
          role="textbox"
          aria-multiline="true"
          aria-label="Corpo da mensagem"
          onPaste={(e) => {
            const html = e.clipboardData?.getData('text/html')
            if (html) {
              e.preventDefault()
              document.execCommand('insertHTML', false, DOMPurify.sanitize(html, { FORBID_TAGS: ['style', 'script', 'img', 'svg'], FORBID_ATTR: ['style', 'class'] }))
            }
          }}
          onDrop={(e) => { if (e.dataTransfer?.files.length) { e.preventDefault(); addFiles(e.dataTransfer.files) } }}
        />
        <Show when={files().length}>
          <div class="composer-files">
            <For each={files()}>
              {(f) => (
                <span class="att" classList={{ pending: f.pending }}>
                  <Icon name="file" size={15} /><span class="att-name">{f.name}</span>
                  <span class="muted small">{f.pending ? 'enviando…' : formatSize(f.size)}</span>
                  <button class="icon-btn sm" aria-label={`Remover ${f.name}`} onClick={() => setFiles((l) => l.filter((x) => x.id !== f.id))}><Icon name="x" size={13} /></button>
                </span>
              )}
            </For>
          </div>
        </Show>
        <div class="composer-foot">
          <button class="btn primary" disabled={sending()} onClick={send}><Icon name="send" size={16} />{sending() ? 'Enviando…' : 'Enviar'}</button>
          <span class="muted small desktop-only">Ctrl/⌘ + Enter envia</span>
          <span class="grow" />
          <button class="icon-btn" title="Descartar" aria-label="Descartar" onClick={() => setComposer(null)}><Icon name="trash" /></button>
        </div>
      </Show>
    </div>
  )
}
