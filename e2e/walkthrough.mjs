// End-to-end walkthrough against a fresh stack (make e2e): setup wizard, login,
// reading, privacy, XSS isolation, attachments, reply, folders, themes, mobile.
import puppeteer from 'puppeteer-core'

import { mkdirSync } from 'node:fs'

const B = process.env.ROOSTY_URL || 'http://localhost:8080'
const SETUP_CODE = process.env.ROOSTY_SETUP_TOKEN || 'roosty-local-setup'
const OUT = new URL('./artifacts/', import.meta.url).pathname
mkdirSync(OUT, { recursive: true })
const CHROME = process.env.CHROME_PATH ||
  (process.platform === 'darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : '/usr/bin/google-chrome')
const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-first-run', '--no-default-browser-check', '--no-sandbox'],
})
let failures = 0
const errors = []
const page = await browser.newPage()
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message))
page.on('dialog', (d) => d.dismiss())
await page.setViewport({ width: 1440, height: 900 })
const shot = (n) => page.screenshot({ path: OUT + n + '.png' })
const wait = (ms) => new Promise((r) => setTimeout(r, ms))
const clickText = async (sel, text) => {
  const els = await page.$$(sel)
  for (const el of els) {
    const t = await el.evaluate((e) => e.textContent.trim())
    if (t.includes(text)) { await el.click(); return true }
  }
  throw new Error(`não achei ${sel} com "${text}"`)
}
const step = async (name, fn) => {
  try { await fn(); console.log('OK  ', name) } catch (e) { failures++; console.log('FAIL', name, '-', e.message); await shot('fail-' + name.replace(/\W+/g, '-')) }
}

await step('setup: abrir assistente', async () => {
  await page.goto(B + '/admin/setup', { waitUntil: 'networkidle2' })
  await page.waitForSelector('#setup-token')
  await page.type('#setup-token', SETUP_CODE)
  await page.type('#setup-pass', 'roosty-admin-123')
  await page.type('#setup-pass2', 'roosty-admin-123')
  await shot('01-setup-conta')
  await clickText('button', 'Continuar')
})
await step('setup: testar servidor', async () => {
  await clickText('button', 'Testar conexão')
  await page.waitForSelector('.test-result', { timeout: 10000 })
  await wait(300)
  await shot('02-setup-servidor')
  await clickText('button', 'Continuar')
})
await step('setup: domínios e branding', async () => {
  await wait(200); await shot('03-setup-dominios'); await clickText('button', 'Continuar')
  await wait(200); await shot('04-setup-branding'); await clickText('button', 'Continuar')
  await wait(200); await shot('05-setup-revisar')
  await clickText('button', 'Concluir configuração')
  await page.waitForNavigation({ waitUntil: 'networkidle2' })
  await page.waitForSelector('.admin-main h1')
  await shot('06-admin-visao-geral')
})
await step('admin: branding', async () => {
  await clickText('.admin-nav button', 'Branding')
  await wait(400); await shot('07-admin-branding')
  await clickText('.admin-nav button', 'Domínios e contas')
  await wait(400); await shot('08-admin-contas')
})
await step('login do usuário', async () => {
  await page.goto(B + '/login', { waitUntil: 'networkidle2' })
  await page.waitForSelector('#login-email')
  await page.type('#login-email', 'marina@roosty.test')
  await page.type('#login-password', 'roosty123')
  await shot('09-login')
  await Promise.all([page.waitForNavigation({ waitUntil: 'networkidle2' }), clickText('button', 'Entrar')])
  await page.waitForSelector('.row', { timeout: 10000 })
  await wait(500); await shot('10-caixa-entrada')
})
await step('ler email com rastreador', async () => {
  await clickText('.row', 'Dokploy')
  await page.waitForSelector('.mail-frame', { timeout: 10000 })
  await wait(800); await shot('11-leitura-privacidade')
  await clickText('button', 'Mostrar imagens')
  await wait(1500); await shot('12-imagens-liberadas')
})
await step('email XSS isolado', async () => {
  await clickText('.row', 'XSS')
  await wait(1200)
  const inner = await page.$eval('.mail-frame', (f) => f.contentDocument.body.innerHTML)
  if (/<script|onerror|javascript:|<svg|<form|<iframe/i.test(inner)) throw new Error('conteúdo ativo encontrado: ' + inner.slice(0, 200))
  await shot('13-xss')
})
await step('email com anexo e imagem embutida', async () => {
  await clickText('.row', 'Ana Ribeiro'); await wait(1000); await shot('14-anexo')
  await clickText('.row', 'Novidades'); await wait(1200); await shot('15-cid')
})
await step('responder', async () => {
  await clickText('.row', 'Ana Ribeiro'); await wait(800)
  await clickText('.reader-actions button', 'Responder')
  await page.waitForSelector('.composer-editor')
  await page.click('.composer-editor')
  await page.keyboard.type('Perfeito, obrigado Ana! Enviado pelo Roosty.')
  await wait(300); await shot('16-composer')
  await clickText('.composer-foot button', 'Enviar')
  await page.waitForFunction(() => !document.querySelector('.composer'), { timeout: 10000 })
  await wait(600); await shot('17-enviado')
})
await step('pasta Enviados', async () => {
  await clickText('.nav-item', 'Enviados'); await wait(1200)
  const n = await page.$$eval('.row', (r) => r.length)
  if (n < 1) throw new Error('Enviados vazio')
  await shot('18-enviados')
})
await step('configurações: temas', async () => {
  await page.click('.account'); await wait(200)
  await clickText('.menu-item', 'Configurações'); await wait(500)
  await shot('19-config-aparencia')
  for (const t of ['Navy', 'Bege', 'Roxo', 'Preto']) {
    await clickText('.theme-card', t); await wait(300)
  }
  await clickText('.settings-nav button', 'Voltar'); await wait(400)
  await clickText('.nav-item', 'Entrada'); await wait(1200)
  await clickText('.row', 'MXroute'); await wait(1200)
  await shot('20-tema-preto')
})
await step('mobile', async () => {
  await page.setViewport({ width: 390, height: 844, isMobile: true, hasTouch: true, deviceScaleFactor: 2 })
  await page.goto(B + '/', { waitUntil: 'networkidle2' })
  await page.waitForSelector('.row'); await wait(500)
  await page.evaluate(() => fetch('/api/prefs', { method: 'PUT', headers: { 'X-Roosty': '1', 'Content-Type': 'application/json' }, body: JSON.stringify({ theme: 'bege' }) }))
  await page.reload({ waitUntil: 'networkidle2' }); await page.waitForSelector('.row'); await wait(500)
  await shot('21-mobile-lista')
  await clickText('.row', 'MXroute'); await wait(1200); await shot('22-mobile-leitura')
  await page.click('.reader-bar .icon-btn'); await wait(300)
  await page.click('.topbar .icon-btn'); await wait(400); await shot('23-mobile-menu')
})

console.log('\nErros no console:', errors.length ? errors : 'nenhum')
await browser.close()
if (failures || errors.length) process.exit(1)
