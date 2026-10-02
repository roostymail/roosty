import { createResource, createSignal, For, onCleanup, onMount, Show } from 'solid-js'
import { api, batch, setUnauthorizedHandler, Summary } from '../lib/api'
import { navigate } from '../lib/router'
import { applyUserTheme, initials, me, setMe, shortDate, toast, updatePrefs, brand } from '../lib/state'
import { THEMES } from '../lib/theme'
import Icon from '../components/Icon'
import { Brand } from '../components/Logo'
import Reader from './mail/Reader'
import Composer, { composer, openComposer } from './mail/Composer'
import Settings from './mail/Settings'
import {
  closeMessage, drawer, loadMailboxes, loadMessages, loading, mailboxes, messages, moveTo, openMessage, openView,
  ROLE_ICON, ROLE_LABEL, roleBox, screen, selected, setDrawer, setFlag, setScreen, total, view,
} from './mail/store'

function TopBar() {
  const [menu, setMenu] = createSignal<'' | 'account' | 'quick'>('')
  const [q, setQ] = createSignal('')
  const search = (e: Event) => {
    e.preventDefault()
    const text = q().trim()
    const v = view()
    openView({ mailbox: v.mailbox, title: text ? `Busca: ${text}` : v.title.replace(/^Busca: .*/, 'Entrada'), text: text || undefined })
  }
  const logout = async () => {
    await api.post('/api/auth/logout').catch(() => {})
    location.href = '/login'
  }
  const close = (e: MouseEvent) => {
    if (!(e.target as HTMLElement).closest('.menu-wrap')) setMenu('')
  }
  onMount(() => document.addEventListener('click', close))
  onCleanup(() => document.removeEventListener('click', close))
  return (
    <header class="topbar">
      <button class="icon-btn mobile-only" aria-label="Menu" onClick={() => setDrawer(true)}><Icon name="menu" size={22} /></button>
      <div class="topbar-brand desktop-only"><Brand /></div>
      <form class="search" onSubmit={search} role="search">
        <Icon name="search" />
        <input id="search" type="search" placeholder="Buscar emails" value={q()} onInput={(e) => setQ(e.currentTarget.value)} aria-label="Buscar emails" />
      </form>
      <span class="grow desktop-only" />
      <div class="menu-wrap desktop-only">
        <button class="icon-btn" aria-label="Ajustes rápidos" onClick={() => setMenu(menu() === 'quick' ? '' : 'quick')}><Icon name="sliders" /></button>
        <Show when={menu() === 'quick'}>
          <div class="menu quick">
            <div class="menu-label">Tema</div>
            <div class="mini-themes">
              <For each={THEMES.filter((t) => !brand()?.themes?.length || brand()!.themes.includes(t.id))}>
                {(t) => (
                  <button class="mini-theme" classList={{ on: (me()?.prefs.theme || brand()?.defaultTheme || 'claro') === t.id }} title={t.name}
                    style={{ background: t.list, 'border-color': t.border }} onClick={() => updatePrefs({ theme: t.id })}>
                    <span style={{ background: t.accent }} />
                  </button>
                )}
              </For>
            </div>
            <div class="menu-label">Densidade</div>
            <div class="seg">
              <button classList={{ on: me()?.prefs.density !== 'compact' }} onClick={() => updatePrefs({ density: 'comfortable' })}>Confortável</button>
              <button classList={{ on: me()?.prefs.density === 'compact' }} onClick={() => updatePrefs({ density: 'compact' })}>Compacta</button>
            </div>
            <button class="menu-item" onClick={() => { setMenu(''); setScreen('settings') }}><Icon name="settings" size={16} />Todas as configurações</button>
          </div>
        </Show>
      </div>
      <div class="menu-wrap">
        <button class="account" aria-label="Conta" onClick={() => setMenu(menu() === 'account' ? '' : 'account')}>
          <span class="avatar accent sm">{initials(me()?.prefs.displayName || me()?.email || '?')}</span>
          <span class="desktop-only">{me()?.email}</span>
          <Icon name="chevronDown" size={15} class="desktop-only" />
        </button>
        <Show when={menu() === 'account'}>
          <div class="menu right">
            <div class="menu-user"><b>{me()?.prefs.displayName || 'Sua conta'}</b><span class="muted small">{me()?.email}</span></div>
            <button class="menu-item" onClick={() => { setMenu(''); setScreen('settings') }}><Icon name="settings" size={16} />Configurações</button>
            <button class="menu-item" onClick={logout}><Icon name="logout" size={16} />Sair</button>
          </div>
        </Show>
      </div>
    </header>
  )
}

function Sidebar() {
  const sys = () => mailboxes().filter((m) => m.role)
  const custom = () => mailboxes().filter((m) => !m.role)
  const isActive = (id: string, flagged = false) => screen() === 'mail' && view().mailbox === id && !!view().flagged === flagged && !view().text
  const [foldersOpen, setFoldersOpen] = createSignal(true)
  const newFolder = async () => {
    const name = window.prompt('Nome da nova pasta')
    if (!name?.trim()) return
    try {
      await batch([['Mailbox/create', { name: name.trim() }, 'c']])
      loadMailboxes()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }
  return (
    <>
      <div class="scrim mobile-only" classList={{ show: drawer() }} onClick={() => setDrawer(false)} />
      <nav class="sidebar" classList={{ open: drawer() }} aria-label="Pastas">
        <div class="mobile-only drawer-brand"><Brand /></div>
        <button class="compose-btn" onClick={() => { setDrawer(false); openComposer('new') }}><Icon name="pencil" />Escrever</button>
        <div class="nav">
          <For each={sys()}>
            {(m) => (
              <>
                <button class="nav-item" classList={{ active: isActive(m.id) }} onClick={() => openView({ mailbox: m.id, title: ROLE_LABEL[m.role!] || m.name })}>
                  <Icon name={ROLE_ICON[m.role!] || 'folder'} /><span class="grow">{ROLE_LABEL[m.role!] || m.name}</span>
                  <Show when={m.role === 'drafts' ? m.total : m.unread}><span class="count">{m.role === 'drafts' ? m.total : m.unread}</span></Show>
                </button>
                <Show when={m.role === 'inbox'}>
                  <button class="nav-item" classList={{ active: isActive(m.id, true) }} onClick={() => openView({ mailbox: m.id, flagged: true, title: 'Favoritos' })}>
                    <Icon name="star" /><span class="grow">Favoritos</span>
                  </button>
                </Show>
              </>
            )}
          </For>
        </div>
        <div class="divider" />
        <div class="nav-section">
          <button class="nav-head" onClick={() => setFoldersOpen(!foldersOpen())}>
            <Icon name={foldersOpen() ? 'chevronDown' : 'chevronRight'} size={14} />PASTAS
          </button>
          <button class="icon-btn sm" aria-label="Nova pasta" title="Nova pasta" onClick={newFolder}>+</button>
        </div>
        <Show when={foldersOpen()}>
          <div class="nav">
            <For each={custom()} fallback={<p class="muted small nav-empty">Nenhuma pasta criada</p>}>
              {(m) => (
                <button class="nav-item" classList={{ active: isActive(m.id), sub: !!m.parent }} onClick={() => openView({ mailbox: m.id, title: m.name })}>
                  <Icon name="folder" /><span class="grow">{m.name}</span>
                  <Show when={m.unread}><span class="count">{m.unread}</span></Show>
                </button>
              )}
            </For>
          </div>
        </Show>
        <span class="grow" />
        <button class="nav-item mobile-only" onClick={() => { setDrawer(false); setScreen('settings') }}><Icon name="settings" /><span class="grow">Configurações</span></button>
        <p class="muted small hint desktop-only">Pressione <kbd>?</kbd> para ver os atalhos</p>
      </nav>
    </>
  )
}

function Row(props: { m: Summary }) {
  const m = () => props.m
  const who = () => {
    const sentBox = roleBox('sent')?.id === m().mailbox || roleBox('drafts')?.id === m().mailbox
    const a = sentBox ? m().to[0] : m().from[0]
    return (sentBox ? 'Para: ' : '') + (a?.name || a?.email || '(desconhecido)')
  }
  return (
    <div class="row" classList={{ unread: m().unread, selected: selected()?.id === m().id }} role="option" aria-selected={selected()?.id === m().id}
      tabIndex={0} onClick={() => openMessage(m())} onKeyDown={(e) => e.key === 'Enter' && openMessage(m())}>
      <span class="avatar">{initials(m().from[0]?.name || m().from[0]?.email || '?')}</span>
      <div class="row-body">
        <div class="row-line">
          <Show when={m().unread}><span class="dot" /></Show>
          <span class="row-from">{who()}</span>
          <span class="row-date">{shortDate(m().date)}</span>
        </div>
        <div class="row-line">
          <span class="row-subject">{m().subject || '(sem assunto)'}</span>
          <Show when={m().hasAttachment}><Icon name="paperclip" size={14} class="muted" /></Show>
          <button class="icon-btn xs star" classList={{ on: m().flagged }} aria-label="Favorito" onClick={(e) => { e.stopPropagation(); setFlag(m(), 'flagged', !m().flagged) }}><Icon name="star" size={14} /></button>
        </div>
        <div class="row-preview">{m().preview}</div>
      </div>
    </div>
  )
}

function MessageList() {
  return (
    <section class="list" aria-label="Mensagens">
      <div class="list-head">
        <div class="grow">
          <h2>{view().title}</h2>
          <span class="muted small">{total() ? `${messages().length} de ${total()}` : loading() ? 'Carregando…' : 'Nenhuma mensagem'}</span>
        </div>
        <div class="seg sm">
          <button classList={{ on: !view().unread }} onClick={() => openView({ ...view(), unread: undefined })}>Todas</button>
          <button classList={{ on: !!view().unread }} onClick={() => openView({ ...view(), unread: true })}>Não lidas</button>
        </div>
        <button class="icon-btn" aria-label="Atualizar" title="Atualizar" onClick={() => { loadMessages(); loadMailboxes() }}><Icon name="refresh" size={17} /></button>
      </div>
      <div class="rows" role="listbox" aria-label={view().title}>
        <Show when={messages().length || loading()} fallback={
          <div class="empty"><Icon name="inbox" size={40} /><p class="empty-title">Nada por aqui</p><p class="muted">{view().text ? 'Nenhum email corresponde à busca.' : 'As mensagens que chegarem aparecem aqui na hora.'}</p></div>
        }>
          <For each={messages()}>{(m) => <Row m={m} />}</For>
          <Show when={loading() && !messages().length}>
            <For each={[1, 2, 3, 4, 5]}>{() => <div class="row skeleton-row"><span class="avatar" /><div class="row-body"><div class="skeleton" /><div class="skeleton short" /></div></div>}</For>
          </Show>
          <Show when={messages().length < total()}>
            <button class="load-more" disabled={loading()} onClick={() => loadMessages(true)}>{loading() ? 'Carregando…' : 'Carregar mais'}</button>
          </Show>
        </Show>
      </div>
      <button class="fab mobile-only" onClick={() => openComposer('new')}><Icon name="pencil" size={20} />Escrever</button>
    </section>
  )
}

const SHORTCUTS: [string, string][] = [
  ['j / k', 'Próxima / anterior'], ['Enter', 'Abrir'], ['u', 'Voltar para a lista'], ['c', 'Escrever'], ['r', 'Responder'],
  ['a', 'Responder a todos'], ['f', 'Encaminhar'], ['e', 'Arquivar'], ['#', 'Excluir'], ['!', 'Spam'], ['s', 'Favorito'], ['/', 'Buscar'], ['?', 'Atalhos'],
]

export default function MailApp() {
  const [help, setHelp] = createSignal(false)
  const [loaded] = createResource(async () => {
    setUnauthorizedHandler(() => navigate('/login', true))
    try {
      const r = await api.get<{ email: string; prefs: any }>('/api/me')
      setMe(r)
      applyUserTheme()
      await loadMailboxes()
      loadMessages()
      return true
    } catch {
      navigate('/login', true)
      return false
    }
  })

  // Real-time updates over Server-Sent Events.
  let es: EventSource | undefined
  onMount(() => {
    es = new EventSource('/api/events')
    es.addEventListener('mailbox', () => {
      loadMailboxes()
      if (view().mailbox === 'INBOX' && screen() === 'mail') loadMessages()
    })
  })
  onCleanup(() => es?.close())

  const move = (d: number) => {
    const list = messages()
    const i = list.findIndex((m) => m.id === selected()?.id)
    const next = list[Math.min(list.length - 1, Math.max(0, i + d))]
    if (next) openMessage(next)
  }
  const onKey = (e: KeyboardEvent) => {
    const t = e.target as HTMLElement
    if (t.closest('input, textarea, [contenteditable], .composer') || e.metaKey || e.ctrlKey || e.altKey) return
    const m = selected()
    const actions: Record<string, () => void> = {
      j: () => move(1), k: () => move(-1), u: closeMessage, c: () => openComposer('new'), '/': () => document.getElementById('search')?.focus(),
      '?': () => setHelp(!help()), Escape: () => setHelp(false),
      r: () => m && openComposer('reply'), a: () => m && openComposer('replyAll'), f: () => m && openComposer('forward'),
      e: () => m && moveTo(m, 'archive', 'Mensagem arquivada'), '#': () => m && moveTo(m, 'destroy', 'Mensagem excluída'),
      '!': () => m && moveTo(m, 'junk', 'Movida para Spam'), s: () => m && setFlag(m, 'flagged', !m.flagged),
    }
    const fn = actions[e.key]
    if (fn) {
      e.preventDefault()
      fn()
    }
  }
  onMount(() => document.addEventListener('keydown', onKey))
  onCleanup(() => document.removeEventListener('keydown', onKey))

  return (
    <Show when={loaded()} fallback={<div class="boot" />}>
      <div class="app" classList={{ reading: !!selected() && screen() === 'mail' }}>
        <TopBar />
        <div class="body">
          <Sidebar />
          <Show when={screen() === 'mail'} fallback={<Settings />}>
            <MessageList />
            <Reader />
          </Show>
        </div>
        <Show when={composer()} keyed>{(_draft) => <Composer />}</Show>
        <Show when={help()}>
          <div class="modal-scrim" onClick={() => setHelp(false)}>
            <div class="modal" role="dialog" aria-label="Atalhos de teclado" onClick={(e) => e.stopPropagation()}>
              <div class="modal-head"><h2>Atalhos de teclado</h2><button class="icon-btn" aria-label="Fechar" onClick={() => setHelp(false)}><Icon name="x" /></button></div>
              <div class="shortcuts">
                <For each={SHORTCUTS}>{([k, d]) => <div class="shortcut"><kbd>{k}</kbd><span>{d}</span></div>}</For>
              </div>
            </div>
          </div>
        </Show>
      </div>
    </Show>
  )
}
