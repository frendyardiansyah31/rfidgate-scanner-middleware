// Pure table logic: filter, sort, paginate. No DOM here.
import { dayKey } from './format.js';

export function applyFilters(items, { search, status, date }) {
  const q = search.trim().toLowerCase();
  return items.filter((it) => {
    if (status && it.status !== status) return false;
    if (date && dayKey(it.date) !== date) return false;
    if (q) {
      return it.itemCode.toLowerCase().includes(q)
        || it.rfid.toLowerCase().includes(q)
        || it.title.toLowerCase().includes(q);
    }
    return true;
  });
}

// Two detections of the same tag closer together than this belong to the same
// "pass" (one table row). The reader polls every ~200 ms, so one pass is a burst
// of detections a few seconds long; a gap longer than this starts a new row.
export const GAP_MS = 60_000;

// Same rule as Go's rfidtag.Normalize (internal/rfidtag/rfidtag.go): remove every
// "SN" (case-sensitive, anywhere, no trim, no uppercasing); if nothing is left,
// keep the original tag. Keep the two in sync.
export function normalizeTag(tag) {
  const n = tag.split('SN').join('');
  return n === '' ? tag : n;
}

const timeOf = (it) => {
  const t = Date.parse(it.date);
  return Number.isNaN(t) ? null : t;
};

// Oldest first: timestamp, then id, then key, so the order never depends on how
// the array happens to be ordered (SSE events can arrive out of order).
function byTimeAsc(a, b) {
  const ta = timeOf(a);
  const tb = timeOf(b);
  if (ta !== tb) return ta - tb;
  const ia = a.id || 0;
  const ib = b.id || 0;
  if (ia !== ib) return ia - ib;
  return a.key < b.key ? -1 : a.key > b.key ? 1 : 0;
}

// One row per tag per pass. Expects already-filtered items, so `times` counts only
// the events that survived the filters. Each result is the newest event of its
// pass (its date, status, title, cover, key) plus `times`. Events with an empty tag
// or an unreadable timestamp cannot be grouped: each stays its own row (times = 1).
export function groupItems(items, gapMs = GAP_MS) {
  const out = [];
  const byTag = new Map();

  for (const it of items) {
    if (typeof it.rfid !== 'string' || it.rfid === '' || timeOf(it) === null) {
      out.push({ ...it, times: 1 });
      continue;
    }
    const tag = normalizeTag(it.rfid);
    if (!byTag.has(tag)) byTag.set(tag, []);
    byTag.get(tag).push(it);
  }

  for (const list of byTag.values()) {
    list.sort(byTimeAsc);
    let count = 0;
    let last = null;
    for (const it of list) {
      if (last && timeOf(it) - timeOf(last) > gapMs) {
        out.push({ ...last, times: count });
        count = 0;
      }
      last = it;
      count += 1;
    }
    out.push({ ...last, times: count });
  }
  return out;
}

export function sortItems(items, { key, dir }) {
  const sign = dir === 'asc' ? 1 : -1;
  const value = key === 'date' ? (it) => Date.parse(it.date) || 0
    : key === 'times' ? (it) => it.times || 0
      : (it) => it[key].toLowerCase();
  return [...items].sort((a, b) => {
    const x = value(a);
    const y = value(b);
    if (x < y) return -sign;
    if (x > y) return sign;
    if (key === 'times') return (Date.parse(b.date) || 0) - (Date.parse(a.date) || 0); // newest first on ties
    return 0;
  });
}

export function paginate(items, page, pageSize) {
  const totalPages = Math.max(1, Math.ceil(items.length / pageSize));
  const current = Math.min(Math.max(page, 0), totalPages - 1);
  const start = current * pageSize;
  return {
    rows: items.slice(start, start + pageSize),
    page: current,
    totalPages,
    total: items.length,
    start,
  };
}

// Window of up to `max` page numbers centred on the current page.
export function pageButtons(page, totalPages, max = 5) {
  if (totalPages <= 1) return [];
  const half = Math.floor(max / 2);
  let start = Math.max(0, page - half);
  const end = Math.min(totalPages, start + max);
  start = Math.max(0, end - max);
  return Array.from({ length: end - start }, (_, i) => start + i);
}
