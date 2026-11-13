const CATALOGUE_BASE = 'https://catalogue.uiii.ac.id';
// Koha OPAC local cover, by biblionumber. The middleware sends image_filename = biblionumber
// only when a cover exists (opac-image.pl answers a 1x1 gif otherwise).
const THUMB_PATH = '/cgi-bin/koha/opac-image.pl?thumbnail=1&biblionumber=';
const LARGE_PATH = '/cgi-bin/koha/opac-image.pl?biblionumber=';
export const PLACEHOLDER = '/assets/placeholder.svg';

const ESC = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

export function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, (c) => ESC[c]);
}

const dateFmt = new Intl.DateTimeFormat('en-GB', {
  day: '2-digit', month: 'short', year: 'numeric',
  hour: '2-digit', minute: '2-digit', hour12: false,
});

// "26 Oct 2026 14:05" (matches the old date-fns 'dd MMM yyyy HH:mm').
export function formatDate(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso ?? '');
  const p = Object.fromEntries(dateFmt.formatToParts(d).map((x) => [x.type, x.value]));
  return `${p.day} ${p.month} ${p.year} ${p.hour}:${p.minute}`;
}

// Local calendar day as yyyy-MM-dd, comparable with <input type="date"> values.
export function dayKey(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${mm}-${dd}`;
}

export function relativeTime(date) {
  const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  return `${Math.floor(minutes / 60)}h ago`;
}

export const thumbSrc = (name) => (name ? `${CATALOGUE_BASE}${THUMB_PATH}${encodeURIComponent(name)}` : PLACEHOLDER);
export const largeSrc = (name) => `${CATALOGUE_BASE}${LARGE_PATH}${encodeURIComponent(name)}`;
