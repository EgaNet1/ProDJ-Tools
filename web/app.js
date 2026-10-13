// ProDJ Tools — interface.
//
// Plain JavaScript, no framework. All state lives in `state`; a few actions
// (open a folder, build, reorder, export) talk to the Go backend and then
// re-render. Lists are rendered from string templates: they hold at most a
// few hundred rows, so redrawing a whole list is both simple and fast.

const $ = (selector, root = document) => root.querySelector(selector)
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)]

// ── Backend ───────────────────────────────────────────────────────────

async function call(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: { 'X-ProDJ': '1', ...(body && { 'Content-Type': 'application/json' }) },
    body: body && JSON.stringify(body),
  })
  const data = await res.json().catch(() => null)
  if (!res.ok) {
    const err = new Error(data?.error || `${res.status} ${res.statusText}`)
    err.log = data?.log
    throw err
  }
  return data
}
const get = path => call('GET', path)
const post = (path, body = {}) => call('POST', path, body)

// ── Language ──────────────────────────────────────────────────────────
//
// Messages come from web/i18n/<language>.json through /api/i18n, with
// English filling any gaps. t('key', a, b) fills the placeholders {0}, {1}.
// Static texts of the page are marked with data-i18n attributes.

const i18n = { lang: 'en', rtl: false, version: '', languages: [], messages: {} }

const t = (key, ...args) =>
  (i18n.messages[key] ?? key).replace(/\{(\d+)\}/g, (m, i) => (i < args.length ? args[i] : m))

// arrow points from one thing to the next in reading order.
const arrow = () => (i18n.rtl ? '←' : '→')

async function loadLanguage() {
  Object.assign(i18n, await get(`/api/i18n?prefer=${encodeURIComponent(navigator.languages.join(','))}`))
  document.documentElement.lang = i18n.lang
  document.documentElement.dir = i18n.rtl ? 'rtl' : 'ltr'
  translatePage()
}

function translatePage() {
  $$('[data-i18n]').forEach(el => { el.textContent = t(el.dataset.i18n) })
  $$('[data-i18n-html]').forEach(el => { el.innerHTML = t(el.dataset.i18nHtml) })
  $$('[data-i18n-title]').forEach(el => { el.title = t(el.dataset.i18nTitle) })
  $$('[data-i18n-label]').forEach(el => el.setAttribute('aria-label', t(el.dataset.i18nLabel)))
  $$('[data-i18n-placeholder]').forEach(el => { el.placeholder = t(el.dataset.i18nPlaceholder) })
  $('#version').textContent = i18n.version
  $('#lang-code').textContent = i18n.lang.toUpperCase()
  $('#lang-menu').innerHTML = i18n.languages.map(l => `<li role="none">
    <button role="menuitemradio" data-lang="${l.code}" aria-checked="${l.code === i18n.lang}" lang="${l.code}"${l.rtl ? ' dir="rtl"' : ''}>${esc(l.name)}</button>
  </li>`).join('')
  showLog(state.log)
  renderVolume()
}

async function setLanguage(code) {
  closeMenus()
  if (code === i18n.lang) return
  await post('/api/settings', { lang: code })
  await loadLanguage()
  // The server writes curve names, transition advice and the cue sheet in
  // the chosen language, so those are fetched again.
  state.curves = await get('/api/curves')
  state.mixKey = ''
  await refresh()
}

// ── State ─────────────────────────────────────────────────────────────

const DEFAULT_WEIGHTS = { harmony: 50, tempo: 50, energy: 50 } // slider units, 50 = normal

const state = {
  view: 'set',
  dir: '',
  recent: [],
  bpmFloor: 88,
  hasBackup: false,
  scan: { running: false, done: 0, total: 0, current: '', errors: [] },
  tracks: new Map(),  // id → track
  loadedDir: null,    // folder whose tracks are arranged into order/bench
  order: [],          // ids in the set, in playing order
  bench: [],          // ids left out of the set
  locked: new Set(),  // ids that keep their position when the set is rebuilt
  selected: null,
  transitions: [],    // transitions[i] is between order[i] and order[i + 1]
  summary: null,
  curves: [],
  curve: 'rise',
  weights: { ...DEFAULT_WEIGHTS },
  overlap: 30,
  direction: 0,       // for "what next": -1 calmer, 0 same, 1 stronger
  mix: null,          // cue points and transition plans for the order
  mixKey: '',         // what the mix plan was computed for
  mixGen: 0,          // bumped whenever tracks are reloaded
  log: [],            // lines of the last export or restore
}

const track = id => state.tracks.get(id)
const serverWeights = () => Object.fromEntries(Object.entries(state.weights).map(([k, v]) => [k, v / 50]))

// The arrangement of each folder is remembered in the browser, so reopening
// a folder brings back the set as it was left.
const storageKey = dir => 'prodj:set:' + dir.toLowerCase()

function saveSet() {
  if (!state.dir) return
  try {
    localStorage.setItem(storageKey(state.dir), JSON.stringify({
      order: state.order,
      bench: state.bench,
      locked: [...state.locked],
      curve: state.curve,
      weights: state.weights,
      overlap: state.overlap,
    }))
  } catch { /* storage unavailable: the set simply is not remembered */ }
}

function loadSet(dir) {
  try { return JSON.parse(localStorage.getItem(storageKey(dir))) } catch { return null }
}

// ── Actions ───────────────────────────────────────────────────────────

async function refresh() {
  const s = await get('/api/state')
  Object.assign(state, { dir: s.dir, recent: s.recent ?? [], bpmFloor: s.bpmFloor, hasBackup: s.hasBackup, scan: s.scan })
  state.tracks = new Map((s.tracks ?? []).map(tr => [tr.id, tr]))
  state.mixGen++

  if (s.scan.running) {
    renderAll()
    clearTimeout(refresh.timer)
    refresh.timer = setTimeout(() => refresh().catch(showError), 300)
    return
  }
  if (state.dir && state.loadedDir !== state.dir) {
    await arrange()
  } else {
    reconcile()
    await evaluate()
  }
  renderAll()
}

// arrange decides the initial order of a freshly opened folder: the set as it
// was last left, or the order of an earlier export, or a new build.
async function arrange() {
  state.loadedDir = state.dir
  state.selected = null
  const ids = [...state.tracks.keys()]
  const saved = loadSet(state.dir)

  if (saved) {
    state.order = saved.order ?? []
    state.bench = saved.bench ?? []
    state.locked = new Set(saved.locked ?? [])
    state.curve = saved.curve ?? state.curve
    state.weights = { ...DEFAULT_WEIGHTS, ...saved.weights }
    state.overlap = saved.overlap ?? state.overlap
    reconcile()
    await evaluate()
  } else if (ids.length && ids.every(id => track(id).number > 0)) {
    state.order = ids.sort((a, b) => track(a).number - track(b).number)
    state.bench = []
    state.locked = new Set()
    await evaluate()
  } else {
    state.order = ids
    state.bench = []
    state.locked = new Set()
    await build()
  }
  state.selected = state.order[0] ?? null
}

// reconcile drops ids of files that disappeared and adds new files to the set.
function reconcile() {
  const exists = id => state.tracks.has(id)
  state.order = state.order.filter(exists)
  state.bench = state.bench.filter(exists)
  state.locked = new Set([...state.locked].filter(exists))
  const placed = new Set([...state.order, ...state.bench])
  state.order.push(...[...state.tracks.keys()].filter(id => !placed.has(id)))
  if (!exists(state.selected)) state.selected = state.order[0] ?? null
}

async function evaluate() {
  if (!state.order.length) {
    state.transitions = []
    state.summary = null
    return
  }
  const r = await post('/api/evaluate', { order: state.order, curve: state.curve, overlap: state.overlap })
  state.transitions = r.transitions
  state.summary = r.summary
}

// build orders the set. With minutes > 0 it chooses tracks from the whole
// folder to fill that slot; otherwise it reorders the current set.
async function build(minutes = 0) {
  const pool = minutes ? [...state.order, ...state.bench] : [...state.order]
  if (!pool.length) return
  const pins = {}
  state.order.forEach((id, i) => { if (state.locked.has(id)) pins[id] = i })

  const r = await post('/api/build', {
    order: pool, pins, minutes,
    curve: state.curve,
    overlap: state.overlap,
    weights: serverWeights(),
  })
  if (minutes) {
    const chosen = new Set(r.order)
    state.bench = pool.filter(id => !chosen.has(id))
  }
  state.order = r.order
  state.transitions = r.transitions
  state.summary = r.summary
  saveSet()
}

async function afterOrderChange() {
  await evaluate()
  saveSet()
  renderView()
}

async function openFolder(dir) {
  closeMenus()
  await post('/api/open', { dir })
  state.loadedDir = null
  await refresh()
}

async function pickFolder() {
  closeMenus()
  const r = await post('/api/pick-folder')
  if (r.dir) {
    state.loadedDir = null
    await refresh()
  }
}

function select(id, scroll = false) {
  state.selected = id
  $$('.track').forEach(li => li.classList.toggle('selected', li.dataset.id === id))
  if (state.summary) drawChart(state.summary)
  if (scroll) $(`.track[data-id="${CSS.escape(id)}"]`)?.scrollIntoView({ block: 'nearest' })
  renderDeck()
}

function step(delta) {
  if (!state.order.length) return
  const i = state.order.indexOf(state.selected)
  const next = Math.min(state.order.length - 1, Math.max(0, i < 0 ? 0 : i + delta))
  select(state.order[next], true)
}

function toggleLock(id) {
  state.locked.has(id) ? state.locked.delete(id) : state.locked.add(id)
  saveSet()
  renderSet()
}

async function moveToBench(id) {
  const i = state.order.indexOf(id)
  if (i < 0) return
  state.order.splice(i, 1)
  state.bench.unshift(id)
  state.locked.delete(id)
  if (state.selected === id) state.selected = state.order[Math.min(i, state.order.length - 1)] ?? null
  await afterOrderChange()
}

async function moveToSet(id, after = null) {
  state.bench = state.bench.filter(x => x !== id)
  state.order = state.order.filter(x => x !== id)
  const at = after === null ? -1 : state.order.indexOf(after)
  at < 0 ? state.order.push(id) : state.order.splice(at + 1, 0, id)
  await afterOrderChange()
}

async function rebuild(message) {
  await build()
  renderView()
  if (message) toast(message)
}

// ── Formatting ────────────────────────────────────────────────────────

const pad = (n, width = 2) => String(n).padStart(width, '0')
const posWidth = () => Math.max(2, String(state.order.length).length)
const signed = v => (v > 0 ? '+' : v < 0 ? '−' : '±') + Math.abs(v).toFixed(1)
const fmtBPM = b => !b ? '—' : Number.isInteger(b) ? String(b) : b.toFixed(1)
const folderName = dir => dir.split(/[\\/]/).filter(Boolean).pop() ?? ''

function fmtTime(seconds) {
  const s = Math.max(0, Math.round(seconds))
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60
  return h ? `${h}:${pad(m)}:${pad(sec)}` : `${m}:${pad(sec)}`
}

const esc = value => String(value ?? '').replace(/[&<>"']/g, c =>
  ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c])

// Camelot wheel colours: 1 is teal, then round through green, yellow,
// orange, red, magenta, violet and blue to 12, cyan.
const keyHue = code => (170 - (parseInt(code) - 1) * 30 + 360) % 360

function keyChip(tr) {
  if (!tr.key) return `<span class="key none" title="${esc(t('key.unknown'))}">—</span>`
  const cls = ['key', tr.key.endsWith('B') && 'major', !tr.keySure && 'unsure'].filter(Boolean).join(' ')
  const title = tr.keySure ? tr.keyName : t('key.unsure', tr.keyName)
  return `<span class="${cls}" style="--h:${keyHue(tr.key)}" title="${esc(title)}">${tr.key}</span>`
}

function meter(energy) {
  const on = Math.round(energy * 9) + 1
  const cells = Array.from({ length: 10 }, (_, i) => `<i${i < on ? ' class="on"' : ''}></i>`).join('')
  return `<span class="meter" title="${esc(t('energy.meter', on))}">${cells}</span>`
}

function waveform(wave) {
  if (!wave?.length) return '<span></span>'
  const w = 128, h = 24, mid = h / 2, dx = w / (wave.length - 1)
  let top = '', bottom = ''
  wave.forEach((v, i) => {
    const x = (i * dx).toFixed(1), y = (v / 255) * (mid - 1)
    top += `${i ? 'L' : 'M'}${x} ${(mid - y).toFixed(1)}`
    bottom = `L${x} ${(mid + y).toFixed(1)}` + bottom
  })
  return `<svg class="wave" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" aria-hidden="true"><path d="${top}${bottom}Z"/></svg>`
}

const icon = {
  play: '<svg viewBox="0 0 16 16"><path class="solid" d="M4.5 2.8v10.4l8.5-5.2z"/></svg>',
  pause: '<svg viewBox="0 0 16 16"><path class="solid" d="M4 2.8h3v10.4H4zM9 2.8h3v10.4H9z"/></svg>',
  lock: '<svg viewBox="0 0 16 16"><rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"/></svg>',
  unlock: '<svg viewBox="0 0 16 16"><rect x="3" y="7" width="10" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 4.9-.7"/></svg>',
  remove: '<svg viewBox="0 0 16 16"><path d="M4 8h8"/></svg>',
  speaker: '<svg viewBox="0 0 16 16"><path class="solid" d="M2 6h2.5L8 3v10L4.5 10H2z"/><path d="M10.5 5.5a3.5 3.5 0 0 1 0 5M12.5 3.5a6.3 6.3 0 0 1 0 9"/></svg>',
  muted: '<svg viewBox="0 0 16 16"><path class="solid" d="M2 6h2.5L8 3v10L4.5 10H2z"/><path d="M10.5 6l4 4M14.5 6l-4 4"/></svg>',
}

// ── Rendering: frame ──────────────────────────────────────────────────

function renderAll() {
  const name = $('#folder-name')
  name.textContent = state.dir || t('folder.none')
  name.parentElement.title = state.dir
  renderRecent($('#recent-menu'))
  renderView()
  renderDeck()
}

function renderView() {
  if (state.view === 'set') renderSet()
  if (state.view === 'mix') renderMix().catch(showError)
  if (state.view === 'tools') renderTools()
  if (state.view === 'export') renderExport()
}

function showView(view) {
  if (!['set', 'mix', 'tools', 'export'].includes(view)) view = 'set'
  state.view = view
  history.replaceState(null, '', `#${view}`) // a reload reopens the same tab
  $$('.tabs button').forEach(b => b.setAttribute('aria-selected', b.dataset.view === view))
  for (const v of ['set', 'mix', 'tools', 'export']) $(`#view-${v}`).hidden = v !== view
  renderView()
}

function renderRecent(list) {
  list.innerHTML = state.recent.length
    ? state.recent.map(dir => `<li><button data-dir="${esc(dir)}" title="${esc(dir)}">${esc(dir)}</button></li>`).join('')
    : `<li class="muted" style="padding:6px 8px">${esc(t('recent.empty'))}</li>`
}

// ── Rendering: set ────────────────────────────────────────────────────

function renderSet() {
  const scanning = state.scan.running
  $('#welcome').hidden = Boolean(state.dir) || scanning
  $('#scanning').hidden = !scanning
  $('#set').hidden = !state.dir || scanning

  if (scanning) return renderScan()
  if (!state.dir) return renderRecent($('#recent-welcome'))

  const rows = []
  state.order.forEach((id, i) => {
    if (i > 0 && state.transitions[i - 1]) rows.push(joinRow(state.transitions[i - 1], i - 1))
    rows.push(trackRow(track(id), i))
  })
  $('#setlist').innerHTML = rows.join('') ||
    `<li class="hint" style="padding:24px 20px">${esc(t(state.tracks.size ? 'set.all_benched' : 'set.no_tracks'))}</li>`

  renderBench()
  renderSide()
  $('#scan-errors').innerHTML = state.scan.errors?.map(e => `<li>${esc(e)}</li>`).join('') ?? ''
  markPlaying()
}

function trackRow(tr, i) {
  const locked = state.locked.has(tr.id)
  const classes = ['track', state.selected === tr.id && 'selected', locked && 'locked'].filter(Boolean).join(' ')
  return `<li class="${classes}" data-id="${tr.id}" draggable="true">
    <span class="grip" title="${esc(t('track.drag'))}"></span>
    <span class="pos">${pad(i + 1, posWidth())}</span>
    <button class="icon" data-act="play" title="${esc(t('track.play'))}">${icon.play}</button>
    <div class="name" title="${esc(tr.file)}"><b>${esc(tr.title)}</b><span>${esc(tr.artist)}</span></div>
    ${waveform(tr.wave)}
    <span class="bpm${tr.bpmSure ? '' : ' unsure'}"${tr.bpmSure ? '' : ` title="${esc(t('bpm.unsure'))}"`}>${fmtBPM(tr.bpm)}</span>
    <span>${keyChip(tr)}</span>
    ${meter(tr.energy)}
    <span class="time">${fmtTime(tr.duration)}</span>
    <button class="icon lock${locked ? ' on' : ''}" data-act="lock" title="${esc(t(locked ? 'track.unlock' : 'track.lock'))}">${locked ? icon.lock : icon.unlock}</button>
    <button class="icon out" data-act="out" title="${esc(t('track.out'))}">${icon.remove}</button>
  </li>`
}

function joinRow(tr, i) {
  return `<li class="join ${tr.grade}" data-join="${i}"${tr.advice ? ` title="${esc(tr.advice)}"` : ''}>
    <span class="link"></span>
    <span class="what"><span class="dot"></span><span class="tempo">${signed(tr.tempoChange)}%</span>·<span>${esc(tr.key)}</span></span>
    <button data-act="join">${esc(t('join.listen'))}</button>
  </li>`
}

function renderScan() {
  const { done, total, current } = state.scan
  $('#scan-bar').style.width = total ? `${(done / total) * 100}%` : '0'
  $('#scan-count').textContent = total ? t('scan.count', done, total) : t('scan.searching')
  $('#scan-file').textContent = current ?? ''
}

function renderBench() {
  $('#bench').hidden = !state.bench.length
  $('#bench-count').textContent = state.bench.length
  $('#bench-list').innerHTML = state.bench.map(id => {
    const tr = track(id)
    return `<li data-id="${id}">
      <div class="name"><b>${esc(tr.title)}</b><span>${esc(tr.artist)}</span></div>
      <span class="bpm">${fmtBPM(tr.bpm)}</span>
      <span>${keyChip(tr)}</span>
      ${meter(tr.energy)}
      <button data-act="in">${esc(t('bench.in'))}</button>
    </li>`
  }).join('')
}

function renderSide() {
  const s = state.summary
  const curve = state.curves.find(c => c.id === state.curve)
  $('#curve-name').textContent = curve ? t('chart.curve', curve.name) : ''
  $('#unlock-all').hidden = !state.locked.size
  $('#build').disabled = state.order.length < 2

  if (!s) {
    $('#stats').innerHTML = ''
    $('#chart').innerHTML = ''
    return
  }
  $('#stats').innerHTML = `
    <div><dt>${esc(t('stats.tracks'))}</dt><dd>${s.count}</dd></div>
    <div><dt>${esc(t('stats.length'))}</dt><dd>${fmtTime(s.length)}<small>${esc(t('stats.mixed', fmtTime(s.mixLength)))}</small></dd></div>
    <div><dt>${esc(t('stats.tempo'))}</dt><dd>${fmtBPM(s.bpmMin)}–${fmtBPM(s.bpmMax)}</dd></div>
    <div class="${s.rough ? 'alert' : ''}"><dt>${esc(t('stats.rough'))}</dt><dd>${s.rough}</dd></div>`
  drawChart(s)
}

function drawChart(s) {
  const W = 300, H = 120, top = 10
  const n = s.energy.length, bw = W / n, gap = n > 48 ? 0.5 : 2
  const current = state.order.indexOf(state.selected)
  const bars = s.energy.map((e, i) => {
    const h = Math.max(2, e * (H - top))
    const tr = track(state.order[i])
    return `<rect x="${(i * bw + gap / 2).toFixed(2)}" y="${(H - h).toFixed(2)}" width="${Math.max(0.5, bw - gap).toFixed(2)}" height="${h.toFixed(2)}"${i === current ? ' class="current"' : ''}><title>${pad(i + 1, posWidth())} ${esc(tr?.title)}</title></rect>`
  }).join('')
  const line = s.target.map((v, i) => `${((i + 0.5) * bw).toFixed(2)},${(H - v * (H - top)).toFixed(2)}`).join(' ')
  $('#chart').innerHTML = `${bars}<polyline points="${line}"/>`
}

// ── Rendering: tools ──────────────────────────────────────────────────

function renderTools() {
  renderCurves()
  renderWeights()
  renderChecks()
  renderNext()
  $('#overlap').value = state.overlap
  $$('#bpm-floor input').forEach(r => { r.checked = Number(r.value) === state.bpmFloor })
}

function renderCurves() {
  $('#curves').innerHTML = state.curves.map(c => {
    const points = c.points.map((v, i) => `${((i / (c.points.length - 1)) * 100).toFixed(1)},${(40 - v * 36).toFixed(1)}`).join(' ')
    return `<button data-curve="${c.id}" aria-pressed="${c.id === state.curve}">
      <svg viewBox="0 0 100 42" preserveAspectRatio="none" aria-hidden="true"><polyline points="${points}"/></svg>
      <b>${esc(c.name)}</b><small>${esc(c.description)}</small>
    </button>`
  }).join('')
}

function renderWeights() {
  $$('[data-weight]').forEach(input => {
    input.value = state.weights[input.dataset.weight]
    input.nextElementSibling.textContent = `×${(input.value / 50).toFixed(1)}`
  })
}

function renderChecks() {
  const list = $('#checks')
  const problems = state.transitions.map((tr, i) => ({ tr, i })).filter(({ tr }) => tr.grade !== 'good')
  if (!problems.length) {
    list.innerHTML = `<li class="all-good">${esc(t(state.order.length > 1 ? 'checks.all_good' : 'checks.too_few'))}</li>`
    return
  }
  const w = posWidth()
  list.innerHTML = problems.map(({ tr, i }) => {
    const a = track(state.order[i]), b = track(state.order[i + 1])
    return `<li class="${tr.grade}" data-pos="${i + 1}" title="${esc(t('checks.show'))}">
      <span class="pair">${pad(i + 1, w)} ${arrow()} ${pad(i + 2, w)} · ${signed(tr.tempoChange)}% · ${esc(tr.key)}</span>
      <p>${esc(a.title)} ${arrow()} ${esc(b.title)}</p>
      ${tr.advice ? `<p>${esc(tr.advice)}</p>` : ''}
    </li>`
  }).join('')
}

async function renderNext() {
  const all = [...state.order, ...state.bench]
  const from = all.includes(state.selected) ? state.selected : all[0]
  const select = $('#next-from')
  select.innerHTML = all.map(id => {
    const tr = track(id), pos = state.order.indexOf(id)
    const label = `${pos < 0 ? t('next.bench') : pad(pos + 1, posWidth())} · ${tr.title} — ${tr.artist}`
    return `<option value="${id}"${id === from ? ' selected' : ''}>${esc(label)}</option>`
  }).join('')
  $$('#next-direction button').forEach(b => b.setAttribute('aria-checked', Number(b.dataset.dir) === state.direction))

  if (!from) {
    $('#suggestions').innerHTML = ''
    drawWheel('')
    return
  }
  drawWheel(track(from).key)
  const list = await post('/api/suggest', { from, direction: state.direction, weights: serverWeights() })
  $('#suggestions').innerHTML = list.map(s => {
    const tr = track(s.id), pos = state.order.indexOf(s.id)
    const where = pos < 0 ? t('next.bench') : t('next.now', pos + 1)
    const shift = s.energyShift > 0.05 ? '↑' : s.energyShift < -0.05 ? '↓' : arrow()
    return `<li>
      <span>${keyChip(tr)}</span>
      <div class="name"><b>${esc(tr.title)}</b><span>${esc(tr.artist)} <span class="where">· ${esc(where)}</span></span></div>
      <span class="tempo">${signed(s.transition.tempoChange)}%</span>
      <span class="rel ${s.transition.grade}">${esc(s.transition.key)}</span>
      <span class="shift" title="${esc(t('next.shift'))}">${shift}</span>
      <button data-insert="${s.id}" data-after="${from}">${esc(t('next.insert'))}</button>
    </li>`
  }).join('')
}

// Keys that mix cleanly with code: neighbours on the same ring and the
// relative key on the other ring.
function compatibleKeys(code) {
  if (!code) return new Set()
  const n = parseInt(code), ring = code.slice(-1), other = ring === 'A' ? 'B' : 'A'
  const wrap = k => ((k + 11) % 12) + 1
  return new Set([`${wrap(n - 1)}${ring}`, `${wrap(n + 1)}${ring}`, `${n}${other}`])
}

const polar = (cx, cy, r, a) => [cx + r * Math.sin(a), cy - r * Math.cos(a)]

function sector(cx, cy, r0, r1, a0, a1) {
  const f = v => v.toFixed(2)
  const [x0, y0] = polar(cx, cy, r1, a0), [x1, y1] = polar(cx, cy, r1, a1)
  const [x2, y2] = polar(cx, cy, r0, a1), [x3, y3] = polar(cx, cy, r0, a0)
  return `M${f(x0)} ${f(y0)}A${r1} ${r1} 0 0 1 ${f(x1)} ${f(y1)}L${f(x2)} ${f(y2)}A${r0} ${r0} 0 0 0 ${f(x3)} ${f(y3)}Z`
}

function drawWheel(current) {
  const counts = {}
  for (const tr of state.tracks.values()) if (tr.key) counts[tr.key] = (counts[tr.key] ?? 0) + 1
  const fits = compatibleKeys(current)
  const cx = 120, cy = 120, slice = Math.PI / 6
  let svg = ''
  for (let n = 1; n <= 12; n++) {
    const a0 = (n % 12) * slice - slice / 2 // 12 at the top, clockwise
    for (const [ring, r0, r1] of [['B', 78, 116], ['A', 40, 78]]) {
      const code = `${n}${ring}`, count = counts[code] ?? 0
      const fill = count
        ? `hsl(${keyHue(code)} 45% ${Math.min(44, 20 + count * 6)}%)`
        : `hsl(${keyHue(code)} 12% 13%)`
      const cls = code === current ? 'current' : fits.has(code) ? 'fits' : ''
      svg += `<path d="${sector(cx, cy, r0, r1, a0, a0 + slice)}" fill="${fill}" class="${cls}"><title>${esc(t('wheel.count', code, count))}</title></path>`
      const [x, y] = polar(cx, cy, (r0 + r1) / 2, a0 + slice / 2)
      svg += `<text x="${x.toFixed(1)}" y="${(y - 5).toFixed(1)}">${code}</text>`
      if (count) svg += `<text class="count" x="${x.toFixed(1)}" y="${(y + 6).toFixed(1)}">${count}</text>`
    }
  }
  $('#wheel').innerHTML = svg
}

// ── Rendering: export ─────────────────────────────────────────────────

function renderExport() {
  const name = $('#ex-name')
  if (name.dataset.dir !== state.dir) {
    name.value = folderName(state.dir).toUpperCase()
    name.dataset.dir = state.dir
  }
  $('#ex-run').disabled = !state.order.length || state.scan.running
  $('#ex-restore').disabled = !state.hasBackup || state.scan.running
}

function showLog(lines, error) {
  state.log = lines ?? []
  const items = state.log.map(l => `<li>${esc(l)}</li>`)
  if (error) items.push(`<li class="error">${esc(error)}</li>`)
  $('#ex-log').innerHTML = items.join('') || `<li class="muted">${esc(t('log.empty'))}</li>`
}

// Destructive buttons ask for a second click instead of a modal dialog.
function confirmed(button, question) {
  if (button.classList.contains('confirm')) {
    clearTimeout(button.confirmTimer)
    button.classList.remove('confirm')
    button.textContent = button.dataset.label
    return true
  }
  button.dataset.label = button.textContent
  button.textContent = question
  button.classList.add('confirm')
  button.confirmTimer = setTimeout(() => {
    button.classList.remove('confirm')
    button.textContent = button.dataset.label
  }, 4000)
  return false
}

async function runExport(button) {
  if (!confirmed(button, t('export.confirm', state.order.length))) return
  stopAudio() // the player must let go of the files before they are renamed
  button.disabled = true
  try {
    const r = await post('/api/export', {
      order: state.order,
      name: $('#ex-name').value,
      rename: $('#ex-rename').checked,
      tags: $('#ex-tags').checked,
      bpmAndKey: $('#ex-bpmkey').checked,
      cues: $('#ex-cues').checked,
      m3u: $('#ex-m3u').checked,
      rekordbox: $('#ex-xml').checked,
    })
    showLog(r.log)
    toast(t('export.done'))
  } catch (err) {
    showLog(err.log, err.message)
    showError(err)
  } finally {
    await refresh()
  }
}

async function runRestore(button) {
  if (!confirmed(button, t('restore.confirm'))) return
  stopAudio()
  button.disabled = true
  try {
    const r = await post('/api/restore')
    showLog(r.log)
    toast(t('restore.done'))
  } catch (err) {
    showLog(err.log, err.message)
    showError(err)
  } finally {
    await refresh()
  }
}

// ── Mix plan ──────────────────────────────────────────────────────────

// clock formats seconds as m:ss.mmm, the way Rekordbox shows positions.
function clock(seconds) {
  const ms = Math.round(seconds * 1000)
  return `${Math.floor(ms / 60000)}:${pad(Math.floor(ms / 1000) % 60)}.${String(ms % 1000).padStart(3, '0')}`
}

const setName = () => $('#ex-name').value.trim() || folderName(state.dir).toUpperCase()

// loadMix fetches cue points and transition plans for the current order,
// reusing the last answer while nothing has changed.
async function loadMix() {
  const key = [state.dir, state.mixGen, setName(), ...state.order].join('|')
  if (state.mixKey !== key) {
    state.mix = await post('/api/mixplan', { order: state.order, name: setName() })
    state.mixKey = key
  }
  return state.mix
}

async function renderMix() {
  const list = $('#mix-transitions'), table = $('#mix-tracks')
  if (!state.order.length) {
    list.innerHTML = `<li class="hint">${esc(t('mix.empty'))}</li>`
    table.innerHTML = ''
    return
  }
  const mix = await loadMix()
  const info = new Map(mix.tracks.map(m => [m.id, m]))
  list.innerHTML = mix.transitions.map((p, i) => transitionCard(p, i, info)).join('') ||
    `<li class="hint">${esc(t('mix.one'))}</li>`
  table.innerHTML = cueHead() + state.order.map((id, i) => cueRow(track(id), info.get(id), i)).join('')
  markPlaying()
}

const cueChip = c => c ? `<i class="cue" style="background:rgb(${c.color.join(',')})">${c.slot}</i>` : ''

function transitionCard(p, i, info) {
  const a = track(state.order[i]), b = track(state.order[i + 1]), w = posWidth()
  const head = `<span class="pair">${pad(i + 1, w)} ${arrow()} ${pad(i + 2, w)}</span>
    <span class="titles">${esc(a.title)}<i>${arrow()}</i>${esc(b.title)}</span>`
  if (!p) {
    return `<li class="mix-card none"><header>${head}</header>
      <p class="note">${esc(t('mix.no_grid'))}</p></li>`
  }
  const ma = info.get(a.id), mb = info.get(b.id)
  const mixIn = mb.cues.find(c => c.slot === 'A')
  return `<li class="mix-card" data-card="${i}">
    <header>${head}
      <span class="meta">${esc(t('mix.meta', p.bars, b.title, signed(p.pitch), ma.bpm.toFixed(2)))}</span>
      <button data-mix="${i}">${esc(t('mix.listen'))}</button>
    </header>
    ${lanes(p, ma, mb)}
    <ol class="steps">
      <li><span class="n">1</span><time>${clock(p.startA.time)}</time><span class="at-bar">${esc(t('mix.bar', p.startA.bar))}</span>
        <span>${t('mix.step1', esc(a.title), esc(b.title), `${cueChip(mixIn)}MIX IN`, `<span class="clock">${clock(p.cueB.time)}</span>`)}</span></li>
      <li><span class="n">2</span><time>${clock(p.swapA.time)}</time><span class="at-bar">${esc(t('mix.bar', p.swapA.bar))}</span>
        <span>${t('mix.step2', esc(b.title), `<span class="clock">${clock(p.swapB.time)}</span>`, p.swapB.bar)}</span></li>
      <li><span class="n">3</span><time>${clock(p.endA.time)}</time><span class="at-bar">${esc(t('mix.bar', p.endA.bar))}</span>
        <span>${t('mix.step3', esc(a.title))}</span></li>
    </ol>
    ${p.note ? `<p class="note">${esc(p.note)}</p>` : ''}
  </li>`
}

// kindAt is the section a bar (0-based) belongs to.
function kindAt(info, bar) {
  if (bar >= info.lastBar) return 'none'
  let kind = 'none'
  for (const s of info.sections ?? []) if (s.bar <= bar) kind = s.kind
  return kind
}

const playheadLine = '<line class="playhead" x1="-10" x2="-10" y1="0" y2="26"/>'

// lanes draws both tracks bar by bar around the transition, aligned the way
// they play together, with the three moments of the plan marked.
function lanes(p, ma, mb) {
  const from = Math.max(1, p.startA.bar - 8), to = p.endA.bar + 4, n = to - from
  const x = bar => ((bar - from) / n) * 1000
  const lane = (info, ownBar) => {
    let rects = ''
    for (let bar = from; bar < to; bar++) {
      const own = ownBar(bar)
      if (own === null || own < 1 || own > info.bars.length) continue
      const h = 3 + (info.bars[own - 1] / 255) * 23
      rects += `<rect class="kind-${kindAt(info, own - 1)}" x="${x(bar).toFixed(1)}" y="${(26 - h).toFixed(1)}" width="${(1000 / n - 1.5).toFixed(1)}" height="${h.toFixed(1)}"/>`
    }
    return rects
  }
  const marks = [['start', p.startA.bar], ['swap', p.swapA.bar], ['end', p.endA.bar]]
  const lines = marks.map(([k, bar]) => `<line class="mark-${k}" x1="${x(bar)}" x2="${x(bar)}" y1="0" y2="26"/>`).join('')
  const svg = content => `<svg viewBox="0 0 1000 26" preserveAspectRatio="none" aria-hidden="true">${content}${lines}${playheadLine}</svg>`
  return `<div class="lanes" data-from="${from}" data-to="${to}">
    <span>A</span>${svg(lane(ma, bar => bar))}
    <span>B</span>${svg(lane(mb, bar => bar >= p.startA.bar ? p.cueB.bar + bar - p.startA.bar : null))}
    <div class="marks">${marks.map(([, bar]) => `<span style="left:${x(bar) / 10}%">${esc(t('mix.bar_short', bar))}</span>`).join('')}</div>
  </div>`
}

const cueHead = () => `<div class="cue-row head"><span>#</span><span>${esc(t('cue.track'))}</span><span>BPM</span><span>${esc(t('cue.precision'))}</span><span>${esc(t('cue.structure'))}</span></div>`

function cueRow(tr, m, i) {
  const [precClass, precText] = !m.jitter ? ['', '—']
    : [m.jitter <= 2 ? 'good' : m.jitter <= 5 ? 'ok' : 'rough', t('cue.ms', m.jitter.toFixed(1))]
  const n = Math.max(1, m.bars.length)
  const strip = m.bars.map((v, b) => {
    const h = 3 + (v / 255) * 19
    return `<rect class="kind-${kindAt(m, b)}" x="${((b / n) * 1000).toFixed(1)}" y="${(22 - h).toFixed(1)}" width="${((1000 / n) * 0.8).toFixed(2)}" height="${h.toFixed(1)}"/>`
  }).join('')
  const cueLines = m.cues.map(c => {
    const x = (((c.bar - 1) / n) * 1000).toFixed(1)
    return `<line x1="${x}" x2="${x}" y1="0" y2="22" style="stroke:rgb(${c.color.join(',')})"/>`
  }).join('')
  let grid = t('cue.no_grid')
  if (m.bpm) grid = t('cue.first_beat', clock(m.firstBeat)) + (m.downbeatSure ? '' : ` · ${t('cue.downbeat_unsure')}`)
  return `<div class="cue-row">
    <span class="pos">${pad(i + 1, posWidth())}</span>
    <div class="name"><b>${esc(tr.title)}</b><span>${esc(tr.artist)}</span>
      <span class="grid-info${m.bpm && !m.downbeatSure ? ' warn' : ''}">${esc(grid)}</span></div>
    <span class="bpm">${m.bpm ? m.bpm.toFixed(2) : '—'}</span>
    <span class="prec ${precClass}" title="${esc(t('cue.precision_title'))}">${esc(precText)}</span>
    <div>
      <svg viewBox="0 0 1000 22" preserveAspectRatio="none" aria-hidden="true">${strip}${cueLines}</svg>
      <div class="cue-list">${m.cues.map(c => `<span>${cueChip(c)}${c.name} <b>${clock(c.time)}</b> · ${esc(t('mix.bar_short', c.bar))}</span>`).join('')}</div>
    </div>
  </div>`
}

// ── Player ────────────────────────────────────────────────────────────
//
// Single tracks stream through an <audio> element. Transitions play through
// the Web Audio API instead: both tracks are decoded in full, so that B can
// start on exactly the right sample of A's bar. An <audio> element starts
// tens of milliseconds late — too late to check a beat-matched transition.

const deck = $('#audio-a')
const player = { id: null, join: null, frame: 0, nodes: [] }
const audioURL = (id, at) => `/audio/${encodeURIComponent(id)}#t=${Math.max(0, at).toFixed(1)}`
let audioContext = null
let master = null // GainNode, see mixer
const decoded = new Map() // id → Promise<AudioBuffer>, the last few tracks

function play(id, at) {
  const tr = track(id)
  if (!tr) return
  if (player.join === null && player.id === id && at === undefined && deck.src) {
    deck.paused ? deck.play().catch(showError) : deck.pause()
    return
  }
  stopAudio()
  player.id = id
  deck.src = audioURL(id, at ?? tr.duration * 0.3) // a third in is usually past the intro
  deck.play().catch(showError)
  renderDeck()
}

function decode(id) {
  if (!decoded.has(id)) {
    const buffer = fetch(`/audio/${encodeURIComponent(id)}`)
      .then(r => r.arrayBuffer())
      .then(data => audioContext.decodeAudioData(data))
    buffer.catch(() => decoded.delete(id))
    decoded.set(id, buffer)
    while (decoded.size > 4) decoded.delete(decoded.keys().next().value) // a decoded track takes ~100 MB
  }
  return decoded.get(id)
}

// mixer starts Web Audio on first use. Both decks of a transition play
// through the master gain, which follows the volume control.
function mixer() {
  if (!audioContext) {
    audioContext = new AudioContext()
    master = audioContext.createGain()
    master.gain.value = loudness()
    master.connect(audioContext.destination)
  }
  return audioContext
}

// channel is one deck of the virtual mixer: source → bass EQ → volume.
function channel(buffer, rate) {
  const source = audioContext.createBufferSource()
  source.buffer = buffer
  source.playbackRate.value = rate
  const bass = audioContext.createBiquadFilter()
  bass.type = 'lowshelf'
  bass.frequency.value = 150
  const volume = audioContext.createGain()
  source.connect(bass).connect(volume).connect(master)
  return { source, bass: bass.gain, volume: volume.gain }
}

// playTransition plays transition i exactly as planned: A from four bars
// before B comes in; B from its MIX IN cue, on A's bar and at A's tempo,
// without bass; at the swap A's bass goes out and B's comes in; A fades out
// by the end of the blend. Tempo matching changes B's pitch slightly, as on
// a turntable.
async function playTransition(i) {
  if (player.join === i) return stopAudio()
  stopAudio()
  const mix = await loadMix()
  const p = mix.transitions[i]
  if (!p) return toast(t('player.no_grid'), 'error')
  const ma = mix.tracks[i], mb = mix.tracks[i + 1]

  mixer()
  player.join = i
  player.id = ma.id
  renderDeck()
  $('#deck-sub').textContent = t('player.loading')
  const [bufferA, bufferB] = await Promise.all([decode(ma.id), decode(mb.id)])
  if (player.join !== i) return // stopped while loading
  await audioContext.resume()

  const bar = 240 / ma.bpm
  const from = Math.max(0, p.startA.time - 4 * bar)
  const t0 = audioContext.currentTime + 0.1
  const at = timeInA => t0 + timeInA - from
  const a = channel(bufferA, 1), b = channel(bufferB, ma.bpm / mb.bpm)

  b.volume.setValueAtTime(0, t0)
  b.volume.setValueAtTime(0, at(p.startA.time))
  b.volume.linearRampToValueAtTime(1, at((p.startA.time + p.swapA.time) / 2))
  b.bass.setValueAtTime(-26, t0)
  b.bass.setValueAtTime(-26, at(p.swapA.time) - 0.01)
  b.bass.linearRampToValueAtTime(0, at(p.swapA.time))
  a.bass.setValueAtTime(0, at(p.swapA.time) - 0.01)
  a.bass.linearRampToValueAtTime(-26, at(p.swapA.time))
  a.volume.setValueAtTime(1, at(p.swapA.time))
  a.volume.linearRampToValueAtTime(0, at(p.endA.time))

  const end = at(p.endA.time) + 4 * bar
  a.source.start(t0, from)
  b.source.start(at(p.startA.time), p.cueB.time)
  a.source.stop(end)
  b.source.stop(end)
  b.source.onended = () => { if (player.join === i) stopAudio() }
  player.nodes = [a.source, b.source]
  renderDeck()

  const tick = () => {
    if (player.join !== i) return
    const timeInA = from + audioContext.currentTime - t0
    const barInA = (timeInA - ma.firstBeat) / bar + 1
    $('#deck-fill').style.width = `${Math.min(100, Math.max(0, ((audioContext.currentTime - t0) / (end - t0)) * 100))}%`
    $('#deck-clock').textContent = `${clock(Math.max(0, timeInA))} · ${t('mix.bar', Math.max(1, Math.floor(barInA)))}`
    const lanesEl = $(`.mix-card[data-card="${i}"] .lanes`)
    if (lanesEl) {
      const x = ((barInA - lanesEl.dataset.from) / (lanesEl.dataset.to - lanesEl.dataset.from)) * 1000
      lanesEl.querySelectorAll('.playhead').forEach(l => { l.setAttribute('x1', x); l.setAttribute('x2', x) })
    }
    player.frame = requestAnimationFrame(tick)
  }
  tick()
}

function stopAudio() {
  cancelAnimationFrame(player.frame)
  for (const node of player.nodes) {
    try { node.stop() } catch { /* already stopped */ }
  }
  player.nodes = []
  $$('.playhead').forEach(l => { l.setAttribute('x1', -10); l.setAttribute('x2', -10) })
  deck.pause()
  deck.removeAttribute('src')
  deck.load() // drops the connection, so the file is no longer held open
  player.join = null
  player.id = null
  renderDeck()
}

function togglePlay() {
  if (player.join !== null) return stopAudio()
  if (player.id) return play(player.id)
  if (state.selected) play(state.selected)
}

function renderDeck() {
  const tr = track(player.id)
  const mixing = player.join !== null
  const playing = mixing || (Boolean(tr) && !deck.paused)
  const button = $('#deck-play')
  button.disabled = !tr && !state.selected
  button.innerHTML = playing ? icon.pause : icon.play
  button.setAttribute('aria-label', t(playing ? 'player.pause' : 'player.play'))
  if (mixing) {
    const b = track(state.order[player.join + 1])
    $('#deck-title').textContent = `${tr?.title} ${arrow()} ${b?.title}`
    $('#deck-sub').textContent = t('player.transition', pad(player.join + 1, posWidth()), pad(player.join + 2, posWidth()))
  } else {
    $('#deck-title').textContent = tr ? `${tr.title} — ${tr.artist}` : t('player.idle')
    $('#deck-sub').textContent = tr ? `${fmtBPM(tr.bpm)} BPM · ${tr.key || '—'} · ${tr.keyName || ''}` : t('player.idle_hint')
  }
  if (!tr) {
    $('#deck-fill').style.width = '0'
    $('#deck-clock').textContent = '0:00'
  }
  markPlaying()
}

function markPlaying() {
  $$('.track').forEach(li => li.classList.toggle('playing', player.join === null && li.dataset.id === player.id))
  $$('.join').forEach(li => {
    const on = Number(li.dataset.join) === player.join
    li.classList.toggle('playing', on)
    li.querySelector('button').textContent = t(on ? 'join.stop' : 'join.listen')
  })
  $$('[data-mix]').forEach(button => {
    const on = Number(button.dataset.mix) === player.join
    button.classList.toggle('on', on)
    button.textContent = t(on ? 'mix.stop' : 'mix.listen')
  })
}

function updateClock() {
  const tr = track(player.id)
  if (!tr || player.join !== null) return
  $('#deck-fill').style.width = `${Math.min(100, (deck.currentTime / tr.duration) * 100)}%`
  $('#deck-clock').textContent = `${fmtTime(deck.currentTime)} / ${fmtTime(tr.duration)}`
}

// ── Volume ────────────────────────────────────────────────────────────
//
// One control for everything that plays: the <audio> element of single
// tracks and the master gain of transitions. The level is remembered in the
// browser.

const volume = { level: 80, muted: false } // level 0–100

// loudness maps the control to gain. Hearing is roughly logarithmic, so a
// squared curve makes the middle of the slider sound like half volume.
const loudness = () => (volume.muted ? 0 : (volume.level / 100) ** 2)

function loadVolume() {
  try { Object.assign(volume, JSON.parse(localStorage.getItem('prodj:volume'))) } catch { /* defaults */ }
}

function setVolume(change) {
  Object.assign(volume, change)
  try { localStorage.setItem('prodj:volume', JSON.stringify(volume)) } catch { /* not remembered */ }
  renderVolume()
}

function renderVolume() {
  deck.volume = loudness()
  if (master) master.gain.setTargetAtTime(loudness(), audioContext.currentTime, 0.015)
  const silent = volume.muted || volume.level === 0
  const slider = $('#volume'), button = $('#mute')
  slider.value = volume.level
  slider.style.setProperty('--level', `${volume.muted ? 0 : volume.level}%`)
  slider.title = `${t('volume.label')}: ${volume.muted ? 0 : volume.level}%`
  button.innerHTML = silent ? icon.muted : icon.speaker
  button.classList.toggle('on', silent)
  button.title = t(silent ? 'volume.unmute' : 'volume.mute')
  button.setAttribute('aria-label', button.title)
}

function toggleMute() {
  if (volume.level === 0) setVolume({ level: 50, muted: false })
  else setVolume({ muted: !volume.muted })
}

// ── Notifications ─────────────────────────────────────────────────────

function toast(message, kind = 'info') {
  const el = $('#toast')
  el.textContent = message
  el.className = `toast ${kind}`
  el.hidden = false
  clearTimeout(toast.timer)
  toast.timer = setTimeout(() => { el.hidden = true }, kind === 'error' ? 7000 : 3500)
}

const showError = err => {
  toast(err?.message ?? String(err), 'error')
  post('/api/log', { message: String(err?.stack ?? err).slice(0, 2000) }).catch(() => {})
}

// Runs an action with its button disabled, reporting errors.
async function busy(button, action) {
  button.disabled = true
  try { await action() } catch (err) { showError(err) } finally { button.disabled = false; renderView() }
}

// ── Folder menu ───────────────────────────────────────────────────────

const menus = [['#folder-menu', '#folder-toggle'], ['#lang-menu', '#lang-toggle']]

function closeMenus() {
  for (const [menu, toggle] of menus) {
    $(menu).hidden = true
    $(toggle).setAttribute('aria-expanded', 'false')
  }
}

function toggleMenu(menu, toggle) {
  const open = $(menu).hidden
  closeMenus()
  $(menu).hidden = !open
  $(toggle).setAttribute('aria-expanded', String(open))
}

// ── Events ────────────────────────────────────────────────────────────

function bindEvents() {
  $$('.tabs button').forEach(b => b.addEventListener('click', () => showView(b.dataset.view)))

  // Folder choice: header menu and welcome screen.
  $('#folder-toggle').addEventListener('click', e => { e.stopPropagation(); toggleMenu('#folder-menu', '#folder-toggle') })
  $('#lang-toggle').addEventListener('click', e => { e.stopPropagation(); toggleMenu('#lang-menu', '#lang-toggle') })
  $('#lang-menu').addEventListener('click', e => {
    const b = e.target.closest('[data-lang]')
    if (b) setLanguage(b.dataset.lang).catch(showError)
  })
  document.addEventListener('click', e => { if (!e.target.closest('.folder, .lang')) closeMenus() })
  document.addEventListener('click', e => {
    if (e.target.closest('[data-act="pick"]')) pickFolder().catch(showError)
    const recent = e.target.closest('[data-dir]')
    if (recent) openFolder(recent.dataset.dir).catch(showError)
  })
  // The author's page opens in the user's own browser, not in the app window.
  $('#github').addEventListener('click', e => {
    e.preventDefault()
    post('/api/github').catch(() => window.open(e.currentTarget.href, '_blank', 'noopener'))
  })

  $('#paste-form').addEventListener('submit', e => {
    e.preventDefault()
    const dir = e.target.dir.value.trim()
    if (dir) openFolder(dir).then(() => { e.target.dir.value = '' }).catch(showError)
  })

  bindSetList()

  $('#bench-list').addEventListener('click', e => {
    const li = e.target.closest('li')
    if (li && e.target.closest('[data-act="in"]')) moveToSet(li.dataset.id).catch(showError)
  })
  $('#build').addEventListener('click', e => busy(e.currentTarget, () => rebuild(t('toast.rebuilt'))))
  $('#unlock-all').addEventListener('click', () => { state.locked.clear(); saveSet(); renderSet() })

  // Tools
  $('#curves').addEventListener('click', e => {
    const b = e.target.closest('[data-curve]')
    if (!b || b.dataset.curve === state.curve) return
    state.curve = b.dataset.curve
    renderCurves()
    const name = state.curves.find(c => c.id === state.curve)?.name
    rebuild(t('toast.rebuilt_curve', name)).catch(showError)
  })
  $$('[data-weight]').forEach(input => {
    input.addEventListener('input', () => { input.nextElementSibling.textContent = `×${(input.value / 50).toFixed(1)}` })
    input.addEventListener('change', () => {
      state.weights[input.dataset.weight] = Number(input.value)
      rebuild(t('toast.rebuilt_weights')).catch(showError)
    })
  })
  $('#weights-reset').addEventListener('click', () => {
    state.weights = { ...DEFAULT_WEIGHTS }
    rebuild(t('toast.weights_reset')).catch(showError)
  })
  $('#overlap').addEventListener('change', e => {
    state.overlap = Math.max(0, Number(e.target.value) || 0)
    afterOrderChange().catch(showError)
  })
  $('#build-slot').addEventListener('click', e => busy(e.currentTarget, async () => {
    const minutes = Number($('#slot-minutes').value)
    if (!(minutes > 0)) throw new Error(t('slot.need_minutes'))
    const pool = state.order.length + state.bench.length
    await build(minutes)
    $('#slot-result').textContent =
      t('slot.result', state.order.length, pool, fmtTime(state.summary?.mixLength ?? 0))
  }))
  $('#next-from').addEventListener('change', e => {
    state.selected = e.target.value
    renderNext().catch(showError)
  })
  $('#next-direction').addEventListener('click', e => {
    const b = e.target.closest('[data-dir]')
    if (!b) return
    state.direction = Number(b.dataset.dir)
    renderNext().catch(showError)
  })
  $('#suggestions').addEventListener('click', e => {
    const b = e.target.closest('[data-insert]')
    if (!b) return
    moveToSet(b.dataset.insert, b.dataset.after)
      .then(() => toast(t('toast.inserted', track(b.dataset.insert).title, track(b.dataset.after).title)))
      .catch(showError)
  })
  $('#checks').addEventListener('click', e => {
    const li = e.target.closest('[data-pos]')
    if (!li) return
    showView('set')
    select(state.order[Number(li.dataset.pos)], true)
  })
  $('#bpm-floor').addEventListener('change', async e => {
    try {
      await post('/api/settings', { bpmFloor: Number(e.target.value) })
      await refresh()
      toast(t('toast.octave'))
    } catch (err) { showError(err) }
  })

  // Mix plan
  $('#mix-transitions').addEventListener('click', e => {
    const button = e.target.closest('[data-mix]')
    if (button) playTransition(Number(button.dataset.mix)).catch(showError)
  })
  $('#mix-copy').addEventListener('click', e => busy(e.currentTarget, async () => {
    const mix = await loadMix()
    await navigator.clipboard.writeText(mix.sheet)
    toast(t('toast.copied'))
  }))
  $('#mix-save').addEventListener('click', e => busy(e.currentTarget, async () => {
    const r = await post('/api/cuesheet', { order: state.order, name: setName() })
    toast(t('toast.saved', r.file))
  }))

  // Export
  $('#ex-run').addEventListener('click', e => runExport(e.currentTarget))
  $('#ex-restore').addEventListener('click', e => runRestore(e.currentTarget))

  // Player
  deck.addEventListener('timeupdate', updateClock)
  deck.addEventListener('play', renderDeck)
  deck.addEventListener('pause', renderDeck)
  deck.addEventListener('ended', () => { if (player.join === null) stopAudio() })
  $('#deck-play').addEventListener('click', togglePlay)
  $('#volume').addEventListener('input', e => setVolume({ level: Number(e.target.value), muted: false }))
  $('#mute').addEventListener('click', toggleMute)
  $('#deck-scrub').addEventListener('click', e => {
    const tr = track(player.id)
    if (!tr || player.join !== null) return
    const box = e.currentTarget.getBoundingClientRect()
    deck.currentTime = ((e.clientX - box.left) / box.width) * tr.duration
  })

  document.addEventListener('keydown', onKey)

  // An open connection tells the program the window is alive; when the
  // window closes the connection drops and the program quits.
  new EventSource('/api/alive')
}

function bindSetList() {
  const list = $('#setlist')

  list.addEventListener('click', e => {
    const act = e.target.closest('[data-act]')?.dataset.act
    const join = e.target.closest('.join')
    if (join) {
      if (act === 'join') playTransition(Number(join.dataset.join)).catch(showError)
      return
    }
    const id = e.target.closest('.track')?.dataset.id
    if (!id) return
    if (act === 'play') return play(id)
    if (act === 'lock') return toggleLock(id)
    if (act === 'out') return moveToBench(id).catch(showError)
    select(id)
  })
  list.addEventListener('dblclick', e => {
    const li = e.target.closest('.track')
    if (li && !e.target.closest('button')) play(li.dataset.id)
  })

  // Drag and drop. A drop on a transition row counts as "before the next track".
  let dragging = null
  const targetOf = e => e.target.closest('.track') ?? e.target.closest('.join')?.nextElementSibling
  const clearMarks = () => $$('.drop-before, .drop-after').forEach(li => li.classList.remove('drop-before', 'drop-after'))

  list.addEventListener('dragstart', e => {
    const li = e.target.closest('.track')
    if (!li) return
    dragging = li.dataset.id
    li.classList.add('dragging')
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', dragging)
  })
  list.addEventListener('dragend', () => {
    dragging = null
    clearMarks()
    $$('.dragging').forEach(li => li.classList.remove('dragging'))
  })
  list.addEventListener('dragover', e => {
    const li = dragging && targetOf(e)
    if (!li) return
    e.preventDefault()
    const after = e.target.closest('.track') && e.clientY > li.getBoundingClientRect().top + li.offsetHeight / 2
    clearMarks()
    li.classList.add(after ? 'drop-after' : 'drop-before')
  })
  list.addEventListener('drop', e => {
    const li = dragging && targetOf(e)
    if (!li) return
    e.preventDefault()
    const after = li.classList.contains('drop-after')
    const moving = dragging
    clearMarks()
    if (li.dataset.id === moving) return
    const order = state.order.filter(id => id !== moving)
    order.splice(order.indexOf(li.dataset.id) + (after ? 1 : 0), 0, moving)
    state.order = order
    afterOrderChange().catch(showError)
  })
}

function onKey(e) {
  if (e.ctrlKey || e.metaKey || e.altKey) return
  // Typing goes to the field. A slider keeps its arrow keys, but Space and M
  // still work after the volume has been dragged.
  if (e.target.closest?.('input, select, textarea')) {
    const slider = e.target.matches('input[type="range"]')
    if (!slider || (e.code !== 'Space' && e.code !== 'KeyM')) return
  }
  if (e.code === 'Space') {
    e.preventDefault()
    e.target.blur?.()
    return togglePlay()
  }
  if (e.code === 'KeyM') return toggleMute()
  if (state.view !== 'set') return
  switch (e.code) {
    case 'ArrowDown': e.preventDefault(); return step(1)
    case 'ArrowUp': e.preventDefault(); return step(-1)
    case 'Enter': return state.selected && play(state.selected)
    case 'KeyL': return state.selected && toggleLock(state.selected)
    case 'Delete':
    case 'Backspace': return state.selected && moveToBench(state.selected).catch(showError)
  }
}

// ── Start ─────────────────────────────────────────────────────────────

// Errors in the page go to the program's log file (prodj.log), where they
// can be found after the fact.
function reportErrors() {
  const send = message => post('/api/log', { message: String(message).slice(0, 2000) }).catch(() => {})
  addEventListener('error', e => send(`${e.message} at ${e.filename}:${e.lineno}`))
  addEventListener('unhandledrejection', e => send(e.reason?.stack ?? e.reason))
}

async function start() {
  reportErrors()
  loadVolume()
  bindEvents()
  await loadLanguage()
  document.body.classList.remove('loading')
  state.curves = await get('/api/curves')
  await refresh()
  showView(location.hash.slice(1))
}

start().catch(showError)
