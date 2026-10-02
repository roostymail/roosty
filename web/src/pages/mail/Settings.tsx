import { createSignal, For, Show } from 'solid-js'
import { brand, me, themeFor, updatePrefs } from '../../lib/state'
import { ACCENTS, contrast, themeById, THEMES, validHex } from '../../lib/theme'
import Icon from '../../components/Icon'
import { setScreen } from './store'

type Section = 'appearance' | 'account' | 'remote' | 'about'

function ThemeCard(props: { id: string }) {
  const t = themeById(props.id)
  const cur = () => themeFor(me()?.prefs).theme === t.id
  return (
    <button class="theme-card" classList={{ on: cur() }} onClick={() => updatePrefs({ theme: t.id })} aria-pressed={cur()}>
      <span class="theme-preview" style={{ background: t.chrome, 'border-color': t.border }}>
        <span class="tp-side" style={{ background: t.chrome }}>
          <i style={{ background: t.accent }} /><i style={{ background: t.surface }} /><i style={{ background: t.surface }} />
        </span>
        <span class="tp-list" style={{ background: t.list, 'border-color': t.border }}>
          <i style={{ background: t.text, opacity: 0.8 }} /><i style={{ background: t.muted, opacity: 0.6 }} /><i style={{ background: t.muted, opacity: 0.4 }} />
        </span>
        <span class="tp-read" style={{ background: t.list }}><b style={{ background: t.paper, 'border-color': t.border }} /></span>
      </span>
      <span class="theme-name">{t.name}<Show when={cur()}><Icon name="check" size={15} /></Show></span>
    </button>
  )
}

export default function Settings() {
  const [section, setSection] = createSignal<Section>('appearance')
  const prefs = () => me()?.prefs || {}
  const allowed = () => THEMES.filter((t) => !brand()?.themes?.length || brand()!.themes.includes(t.id))
  const accent = () => prefs().accent || ''
  const [custom, setCustom] = createSignal(accent())
  const bg = () => themeById(themeFor(prefs()).theme).list
  const effAccent = () => accent() || themeFor(prefs()).accent || themeById(themeFor(prefs()).theme).accent
  const ratio = () => contrast(effAccent(), bg())
  const nav: [Section, string, string][] = [['appearance', 'Aparência', 'palette'], ['account', 'Conta e assinatura', 'users'], ['remote', 'Conteúdo remoto', 'shield'], ['about', 'Sobre', 'alert']]

  return (
    <section class="settings">
      <nav class="settings-nav">
        <button class="nav-item back" onClick={() => setScreen('mail')}><Icon name="arrowLeft" />Voltar para o email</button>
        <For each={nav}>
          {([id, label, icon]) => (
            <button class="nav-item" classList={{ active: section() === id }} onClick={() => setSection(id)}><Icon name={icon} />{label}</button>
          )}
        </For>
      </nav>
      <div class="settings-body">
        <Show when={section() === 'appearance'}>
          <h1>Aparência</h1>
          <h3>Tema</h3>
          <div class="theme-grid"><For each={allowed()}>{(t) => <ThemeCard id={t.id} />}</For></div>
          <h3>Cor de destaque</h3>
          <p class="muted">Vale para qualquer tema. Sem escolha, cada tema usa a própria cor.</p>
          <div class="accent-row">
            <button class="swatch auto" classList={{ on: !accent() }} onClick={() => updatePrefs({ accent: '' })}>Padrão</button>
            <For each={ACCENTS}>{(c) => <button class="swatch" classList={{ on: accent().toLowerCase() === c.toLowerCase() }} style={{ background: c }} aria-label={`Cor ${c}`} onClick={() => updatePrefs({ accent: c })} />}</For>
            <label class="hex">
              <input type="color" value={validHex(custom()) ? custom() : effAccent()} onInput={(e) => { setCustom(e.currentTarget.value); updatePrefs({ accent: e.currentTarget.value }) }} aria-label="Cor personalizada" />
              <input id="accent-hex" value={custom()} placeholder="#RRGGBB" onChange={(e) => { const v = e.currentTarget.value.trim(); setCustom(v); if (validHex(v)) updatePrefs({ accent: v }) }} />
            </label>
          </div>
          <p class="small" classList={{ warn: ratio() < 3 }}>
            Contraste com o fundo: {ratio().toFixed(1)}:1 {ratio() >= 4.5 ? '· ótimo' : ratio() >= 3 ? '· bom para ícones e botões' : '· baixo, escolha uma cor mais forte'}
          </p>
          <h3>Densidade</h3>
          <div class="seg">
            <button classList={{ on: prefs().density !== 'compact' }} onClick={() => updatePrefs({ density: 'comfortable' })}>Confortável</button>
            <button classList={{ on: prefs().density === 'compact' }} onClick={() => updatePrefs({ density: 'compact' })}>Compacta</button>
          </div>
          <h3>Tamanho do texto</h3>
          <div class="seg">
            <button classList={{ on: prefs().fontSize === 'small' }} onClick={() => updatePrefs({ fontSize: 'small' })}>Pequeno</button>
            <button classList={{ on: !prefs().fontSize || prefs().fontSize === 'normal' }} onClick={() => updatePrefs({ fontSize: 'normal' })}>Normal</button>
            <button classList={{ on: prefs().fontSize === 'large' }} onClick={() => updatePrefs({ fontSize: 'large' })}>Grande</button>
          </div>
        </Show>
        <Show when={section() === 'account'}>
          <h1>Conta e assinatura</h1>
          <label class="field">
            <span>Nome de exibição</span>
            <input id="display-name" value={prefs().displayName || ''} placeholder="Como seu nome aparece para quem recebe" onChange={(e) => updatePrefs({ displayName: e.currentTarget.value.trim() })} />
          </label>
          <label class="field">
            <span>Endereço</span>
            <input value={me()?.email} disabled />
          </label>
          <label class="field">
            <span>Assinatura</span>
            <textarea id="signature" rows={5} value={prefs().signature || ''} placeholder={'Seu nome\nseusite.com'} onChange={(e) => updatePrefs({ signature: e.currentTarget.value })} />
          </label>
          <p class="muted small">Alterações são salvas automaticamente.</p>
        </Show>
        <Show when={section() === 'remote'}>
          <h1>Conteúdo remoto</h1>
          <p class="muted">Imagens remotas ficam ocultas por padrão e, quando liberadas, passam pelo proxy do servidor, que esconde seu IP. Estes remetentes sempre têm as imagens exibidas:</p>
          <div class="list-box">
            <For each={prefs().trustedSenders || []} fallback={<p class="muted small">Nenhum remetente na lista.</p>}>
              {(s) => (
                <div class="list-box-row"><span class="grow">{s}</span>
                  <button class="btn sm" onClick={() => updatePrefs({ trustedSenders: (prefs().trustedSenders || []).filter((x) => x !== s) })}>Remover</button>
                </div>
              )}
            </For>
          </div>
        </Show>
        <Show when={section() === 'about'}>
          <h1>Sobre</h1>
          <p>{brand()?.name || 'Roosty Mail'} usa o Roosty Mail {brand()?.version}, um webmail open source sob a licença AGPL-3.0.</p>
          <p><a href="https://roosty.dev" target="_blank" rel="noopener noreferrer">roosty.dev</a> · <a href="https://github.com/roostymail/roosty" target="_blank" rel="noopener noreferrer">código-fonte</a></p>
        </Show>
      </div>
    </section>
  )
}
