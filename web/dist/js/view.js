// DOM rendering. Receives plain data, returns nothing; all strings are escaped.
import { icon } from './icons.js';
import { esc, formatDate, relativeTime, thumbSrc, largeSrc } from './format.js';

const $ = (id) => document.getElementById(id);

const COLUMNS = [
  { label: 'No' },
  { label: 'Date / Time', sort: 'date' },
  { label: 'Item Code', sort: 'itemCode' },
  { label: 'RFID Tag EPC', sort: 'rfid' },
  { label: 'Cover' },
  { label: 'Title', sort: 'title' },
  { label: 'Status' },
  { label: 'Times', sort: 'times' },
];
const SKELETON_WIDTHS = [30, 90, 80, 70, 44, 160, 90, 40];

// ─── Status pills + metrics ──────────────────────────────────────────────────

function pill(ok, text, pulse = false) {
  const tone = ok ? 'bg-emerald-50 text-emerald-900 border-emerald-300' : 'bg-rose-50 text-rose-900 border-rose-300';
  const dot = ok ? `bg-emerald-600${pulse ? ' animate-pulse' : ''}` : 'bg-rose-600';
  return `<span class="inline-flex items-center gap-1.5 text-sm font-semibold px-3 py-1 rounded-full border ${tone}">
    <span class="w-2 h-2 rounded-full ${dot}"></span>${text}</span>`;
}

export function renderStatus(s) {
  const middleware = s.restOk || s.streamLive;
  $('status-pills').innerHTML =
    pill(middleware, `Middleware ${middleware ? 'Connected' : 'Not Connected'}`, middleware)
    + pill(s.slimsOnline, `SLiMS ILS ${s.slimsOnline ? 'Online' : 'Offline'}`)
    + pill(s.streamLive, s.streamLive ? 'Live' : 'Reconnecting…', s.streamLive)
    + (s.lastUpdatedAt
      ? `<span class="text-sm font-medium text-slate-600 pl-1">Last update: <span class="font-semibold text-slate-800">${relativeTime(s.lastUpdatedAt)}</span></span>`
      : '');

  $('offline-banner').hidden = s.browserOnline && middleware;

  const btn = $('refresh-btn');
  btn.disabled = s.loading;
  $('refresh-icon').firstElementChild?.classList.toggle('animate-spin', s.loading);
}

function metricCard({ label, labelClass = 'text-slate-600', value, valueClass = 'text-slate-900', unit, unitClass = 'text-slate-600', cardClass = 'border-slate-200', dot = '' }) {
  return `<div class="bg-white p-3 rounded-lg border ${cardClass} shadow-sm flex flex-col justify-between"><div>
    <div class="flex items-center justify-between">
      <span class="text-xs font-semibold uppercase tracking-wider ${labelClass}">${label}</span>${dot}
    </div>
    <div class="mt-1 flex items-baseline gap-1.5">
      <span class="text-xl font-extrabold ${valueClass} tabular-nums">${value}</span>
      <span class="text-xs font-medium ${unitClass}">${unit}</span>
    </div></div></div>`;
}

export function renderMetrics(s, items) {
  const permitted = items.filter((i) => i.status === 'Permitted').length;
  const alarms = items.length - permitted;
  const v = (n) => (s.loading && items.length === 0 ? '—' : n);
  const dot = (ok) => `<span class="w-2 h-2 rounded-full ${ok ? 'bg-emerald-500' : 'bg-rose-500'}"></span>`;

  $('metrics').innerHTML =
    metricCard({ label: 'Total Gate Detections', value: v(items.length), unit: 'items logged' })
    + metricCard({
      label: 'Permitted Exits', labelClass: 'text-emerald-800', value: v(permitted), valueClass: 'text-emerald-700',
      unit: 'authorized loans', dot: '<span aria-hidden="true" class="w-2 h-2 rounded-full bg-emerald-500"></span>',
    })
    + metricCard({
      label: 'Security Triggers', labelClass: 'text-rose-800', value: v(alarms), valueClass: 'text-rose-700',
      unit: 'alarms raised', unitClass: 'text-rose-900 font-semibold', cardClass: 'border-rose-200 bg-rose-50/20',
      dot: '<span aria-hidden="true" class="w-2 h-2 rounded-full bg-rose-600 animate-ping"></span>',
    })
    + `<div class="bg-white p-3 rounded-lg border border-slate-200 shadow-sm flex flex-col justify-between"><div>
        <span class="text-xs font-semibold uppercase tracking-wider text-slate-600">System Status</span>
        <div class="mt-1 flex items-center gap-1.5">${dot(s.restOk || s.streamLive)}<span class="text-sm font-bold text-slate-900">Middleware</span></div>
        <div class="mt-1 flex items-center gap-1.5">${dot(s.slimsOnline)}<span class="text-sm font-bold text-slate-900">SLiMS ILS</span></div>
      </div></div>`;
}

// ─── Table ───────────────────────────────────────────────────────────────────

export function renderHead(sort) {
  $('table-head').innerHTML = COLUMNS.map((c) => {
    let inner = esc(c.label);
    if (c.sort) {
      const active = sort.key === c.sort;
      const ic = active ? (sort.dir === 'asc' ? 'arrow-up' : 'arrow-down') : 'arrow-up-down';
      inner = `<button type="button" data-sort="${c.sort}" class="inline-flex items-center gap-1 uppercase tracking-wider text-xs font-bold text-slate-700 hover:text-slate-900 focus:outline-none">
        ${esc(c.label)}${icon(ic, `w-4 h-4 ${active ? 'text-slate-600' : 'text-slate-400'}`)}</button>`;
    }
    return `<th class="py-4 px-4 uppercase tracking-wider text-xs font-bold text-slate-700 whitespace-nowrap">${inner}</th>`;
  }).join('');
}

function statusBadge(status) {
  const ok = status === 'Permitted';
  const tone = ok ? 'bg-emerald-50 text-emerald-900 border-emerald-300' : 'bg-rose-100 text-rose-900 border-rose-400';
  return `<span class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-sm font-bold border shadow-xs ${tone}">
    ${ok ? icon('check-circle', 'w-4 h-4 text-emerald-700') : icon('alert-triangle', 'w-4 h-4 text-rose-700')}${esc(status)}</span>`;
}

function row(item, no, fresh) {
  const alarm = item.status === 'Not Permitted';
  const cls = [
    alarm ? 'bg-rose-50/30 hover:bg-rose-50/60' : 'hover:bg-slate-50/80',
    'transition-colors',
    fresh ? 'row-new' : '',
  ].join(' ');
  const cover = `<img src="${esc(thumbSrc(item.imageName))}" alt="" loading="lazy"
      ${item.imageName ? `data-large="${esc(largeSrc(item.imageName))}"` : ''}
      class="w-10 h-14 object-cover rounded-md border border-gray-100 ${item.imageName ? 'cursor-zoom-in' : ''}" />`;
  return `<tr class="${cls}">
    <td class="py-3 px-4 text-slate-800 tabular-nums">${no}</td>
    <td class="py-3 px-4 text-slate-800 whitespace-nowrap">${esc(formatDate(item.date))}</td>
    <td class="py-3 px-4"><span class="font-mono font-bold text-slate-900 text-sm tracking-tight">${esc(item.itemCode) || '—'}</span></td>
    <td class="py-3 px-4"><span class="font-mono text-xs text-slate-700 font-semibold max-w-[170px] truncate block" title="${esc(item.rfid)}">${esc(item.rfid)}</span></td>
    <td class="py-3 px-4">${cover}</td>
    <td class="py-3 px-4"><span class="font-bold text-slate-900 line-clamp-2 max-w-sm block" title="${esc(item.title)}">${esc(item.title) || '—'}</span></td>
    <td class="py-3 px-4">${statusBadge(item.status)}</td>
    <td class="py-3 px-4 text-slate-900 font-bold tabular-nums">${item.times ?? 1}</td>
  </tr>`;
}

function skeletonRows() {
  const cells = SKELETON_WIDTHS.map((w) => `<td class="px-4 py-3"><div class="h-3 bg-slate-100 rounded animate-pulse" style="width:${w}px"></div></td>`).join('');
  return Array.from({ length: 7 }, () => `<tr>${cells}</tr>`).join('');
}

export function renderTable(s, pageInfo) {
  const empty = s.loading && s.itemCount === 0;
  $('table-body').innerHTML = empty
    ? skeletonRows()
    : pageInfo.rows.length === 0
      ? `<tr><td colspan="${COLUMNS.length}" class="py-12 px-4 text-center text-slate-500">No records match the current filters.</td></tr>`
      : pageInfo.rows.map((it, i) => row(it, pageInfo.start + i + 1, s.fresh.has(it.key))).join('');

  $('range-label').innerHTML = pageInfo.total === 0
    ? 'No events'
    : `Showing <span class="font-bold text-slate-900 tabular-nums">${pageInfo.rows.length}</span>
       of <span class="font-bold text-slate-900 tabular-nums">${pageInfo.total}</span> passes`;
}

// Pagination disabled (kept for reference):
/*
export function renderPager(pageInfo, buttons) {
  const nav = 'relative inline-flex items-center px-3 py-2 border border-slate-300 bg-white text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:bg-slate-100 disabled:text-slate-400 disabled:cursor-not-allowed focus:z-20 focus:outline-none focus:ring-2 focus:ring-blue-600';
  const prevOff = pageInfo.page <= 0;
  const nextOff = pageInfo.page >= pageInfo.totalPages - 1;
  $('pager').innerHTML =
    `<button type="button" data-page="${pageInfo.page - 1}" ${prevOff ? 'disabled' : ''} class="${nav} rounded-l-md"><span class="sr-only">Previous Page</span>${icon('chevron-left', 'h-4 w-4')}</button>`
    + buttons.map((p) => (p === pageInfo.page
      ? `<button type="button" aria-current="page" data-page="${p}" class="relative z-10 inline-flex items-center px-4 py-2 border border-blue-600 bg-blue-700 text-sm font-bold text-white focus:z-20">${p + 1}</button>`
      : `<button type="button" data-page="${p}" class="relative inline-flex items-center px-4 py-2 border border-slate-300 bg-white text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:z-20 focus:outline-none focus:ring-2 focus:ring-blue-600">${p + 1}</button>`)).join('')
    + `<button type="button" data-page="${pageInfo.page + 1}" ${nextOff ? 'disabled' : ''} class="${nav} rounded-r-md"><span class="sr-only">Next Page</span>${icon('chevron-right', 'h-4 w-4')}</button>`;
}
*/

// ─── Error state ─────────────────────────────────────────────────────────────

export function renderError(message) {
  const box = $('error-state');
  const show = Boolean(message);
  box.hidden = !show;
  $('filters').hidden = show;
  $('table-section').hidden = show;
  if (!show) return;
  box.innerHTML = `<section class="bg-white rounded-lg border border-slate-200 shadow-sm">
    <div class="flex flex-col items-center justify-center py-16 gap-4 text-center">
      <div class="p-4 bg-rose-50 rounded-full text-rose-500">${icon('alert-triangle', 'w-8 h-8')}</div>
      <div>
        <p class="text-slate-900 font-bold text-lg">Failed to load data</p>
        <p class="text-sm text-slate-600 mt-1 max-w-sm">${esc(message)}</p>
      </div>
      <button type="button" id="retry-btn" class="flex items-center gap-2 px-5 py-2.5 bg-blue-700 hover:bg-blue-800 text-white text-sm font-semibold rounded-lg shadow-sm transition-colors">
        ${icon('refresh', 'w-4 h-4')}Try Again</button>
    </div></section>`;
}

// ─── Cover hover preview (single fixed element, so the table can't clip it) ──

const PREVIEW_WIDTH = 220;
const PREVIEW_HEIGHT = Math.round((PREVIEW_WIDTH * 4) / 3);
const PREVIEW_GAP = 12;
let preview = null;

export function showCoverPreview(img) {
  if (!preview) {
    preview = document.createElement('img');
    preview.alt = '';
    preview.className = 'fixed z-50 aspect-[3/4] object-cover rounded-lg shadow-2xl border border-gray-200 pointer-events-none bg-white';
    preview.style.width = `${PREVIEW_WIDTH}px`;
    preview.onerror = () => { preview.onerror = null; preview.src = '/assets/placeholder.svg'; };
    document.body.appendChild(preview);
  }
  const rect = img.getBoundingClientRect();
  const left = rect.right + PREVIEW_GAP + PREVIEW_WIDTH <= window.innerWidth
    ? rect.right + PREVIEW_GAP
    : Math.max(rect.left - PREVIEW_WIDTH - PREVIEW_GAP, PREVIEW_GAP);
  const top = Math.min(Math.max(rect.top, PREVIEW_GAP), window.innerHeight - PREVIEW_HEIGHT - PREVIEW_GAP);
  preview.onerror = () => { preview.onerror = null; preview.src = '/assets/placeholder.svg'; };
  preview.src = img.dataset.large;
  preview.style.top = `${top}px`;
  preview.style.left = `${left}px`;
  preview.hidden = false;
}

export function hideCoverPreview() {
  if (preview) preview.hidden = true;
}
