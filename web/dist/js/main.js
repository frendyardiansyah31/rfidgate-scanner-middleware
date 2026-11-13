import { fetchLogs, openStream } from './api.js';
// Pagination disabled: paginate / pageButtons / renderPager are no longer used.
import { applyFilters, groupItems, sortItems } from './table.js';
// import { applyFilters, groupItems, sortItems, paginate, pageButtons } from './table.js';
import { hydrateIcons } from './icons.js';
import { PLACEHOLDER } from './format.js';
import {
  renderStatus, renderHead, renderTable, renderError, // renderPager (pagination disabled)
  showCoverPreview, hideCoverPreview,
} from './view.js';

const MAX_ITEMS = 1000;       // cap memory; the DB keeps everything
const RESYNC_MS = 60_000;     // safety net in case an SSE event was missed
const FRESH_MS = 2_500;       // how long a live row stays highlighted

const byKey = new Map();
const state = {
  items: [],
  loading: true,
  error: null,
  restOk: true,
  streamLive: false,
  browserOnline: navigator.onLine,
  search: '',
  status: '',
  date: '',
  sort: { key: 'date', dir: 'desc' },
  // page: 0, // pagination disabled
  pageSize: 50, // "Rows shown" (default; keep in sync with the selected <option> in index.html)
  fresh: new Set(),
};

const $ = (id) => document.getElementById(id);

// ─── Rendering (coalesced to one per frame) ──────────────────────────────────

let queued = false;
function render() {
  if (queued) return;
  queued = true;
  requestAnimationFrame(() => {
    queued = false;
    draw();
  });
}

function draw() {
  renderStatus(state);
  renderError(state.error && state.items.length === 0 ? state.error : null);

  // The table shows grouped passes (one row per tag per pass).
  const filtered = applyFilters(state.items, state);
  const grouped = groupItems(filtered);
  const sorted = sortItems(grouped, state.sort);
  // "Rows shown": just the first N rows of the sorted list, no pages.
  const pageInfo = { rows: sorted.slice(0, state.pageSize), start: 0, total: sorted.length };
  // Pagination disabled:
  // const pageInfo = paginate(sorted, state.page, state.pageSize);
  // state.page = pageInfo.page;

  renderHead(state.sort);
  renderTable({ ...state, itemCount: state.items.length }, pageInfo);
  // renderPager(pageInfo, pageButtons(pageInfo.page, pageInfo.totalPages));
}

// ─── Data ────────────────────────────────────────────────────────────────────

function merge(incoming, { live = false } = {}) {
  for (const item of incoming) {
    if (byKey.has(item.key)) continue;
    byKey.set(item.key, item);
    if (live) {
      state.fresh.add(item.key);
      setTimeout(() => { state.fresh.delete(item.key); }, FRESH_MS);
    }
  }
  state.items = [...byKey.values()]
    .sort((a, b) => Date.parse(b.date) - Date.parse(a.date))
    .slice(0, MAX_ITEMS);
  if (byKey.size > state.items.length) {
    byKey.clear();
    state.items.forEach((it) => byKey.set(it.key, it));
  }
}

async function load({ quiet = false } = {}) {
  if (!quiet) {
    state.loading = true;
    render();
  }
  try {
    merge(await fetchLogs());
    state.restOk = true;
    state.error = null;
  } catch (err) {
    console.error('[dashboard] load failed', err);
    state.restOk = false;
    state.error = err instanceof Error ? err.message : 'Terjadi kesalahan saat memuat data.';
  } finally {
    state.loading = false;
    render();
  }
}

// ─── Wiring ──────────────────────────────────────────────────────────────────

function bindControls() {
  // Pagination disabled: there is no page to reset.
  // const resetPage = () => { state.page = 0; };

  $('search-input').addEventListener('input', (e) => { state.search = e.target.value; render(); });
  $('status-filter').addEventListener('change', (e) => { state.status = e.target.value; render(); });
  $('date-filter').addEventListener('change', (e) => { state.date = e.target.value; render(); });
  $('rows-per-page').addEventListener('change', (e) => { state.pageSize = Number(e.target.value); render(); });
  $('reset-btn').addEventListener('click', () => {
    state.search = ''; state.status = ''; state.date = '';
    $('search-input').value = ''; $('status-filter').value = ''; $('date-filter').value = '';
    render();
  });
  $('refresh-btn').addEventListener('click', load);

  $('table-head').addEventListener('click', (e) => {
    const key = e.target.closest('[data-sort]')?.dataset.sort;
    if (!key) return;
    state.sort = state.sort.key === key
      ? { key, dir: state.sort.dir === 'asc' ? 'desc' : 'asc' }
      : { key, dir: key === 'date' || key === 'times' ? 'desc' : 'asc' };
   
    render();
  });

  // Pagination disabled:
  // $('pager').addEventListener('click', (e) => {
  //   const btn = e.target.closest('[data-page]');
  //   if (!btn || btn.disabled) return;
  //   state.page = Number(btn.dataset.page);
  //   render();
  // });

  $('error-state').addEventListener('click', (e) => {
    if (e.target.closest('#retry-btn')) load();
  });

  const body = $('table-body');
  body.addEventListener('mouseover', (e) => {
    const img = e.target.closest('img[data-large]');
    if (img) showCoverPreview(img);
  });
  body.addEventListener('mouseout', (e) => {
    if (e.target.closest('img[data-large]')) hideCoverPreview();
  });
  // 'error' does not bubble, so listen in the capture phase.
  body.addEventListener('error', (e) => {
    const img = e.target;
    if (img.tagName === 'IMG' && !img.src.endsWith(PLACEHOLDER)) img.src = PLACEHOLDER;
  }, true);

  window.addEventListener('online', () => { state.browserOnline = true; load(); });
  window.addEventListener('offline', () => { state.browserOnline = false; render(); });
}

function start() {
  hydrateIcons();
  $('year').textContent = new Date().getFullYear();
  bindControls();
  render();

  load();

  let hasOpened = false;
  openStream({
    onOpen: () => {
      state.streamLive = true;
      // On a reconnect, refetch to fill the gap; merge() de-duplicates.
      if (hasOpened) load({ quiet: true });
      hasOpened = true;
      render();
    },
    onEntry: (item) => {
      merge([item], { live: true });
      render();
    },
    onError: () => {
      state.streamLive = false;
      render();
    },
  });

  setInterval(() => { if (navigator.onLine) load({ quiet: true }); }, RESYNC_MS);
}

start();
