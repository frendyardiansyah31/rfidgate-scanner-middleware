// Same-origin calls: the dashboard is served by the middleware itself, so no
// base URL and no CORS.

// Server record (snake_case) -> dashboard item. status: 0 = allow, 1 = alarm.
export function mapRecord(rec) {
  const date = rec.created_at;
  return {
    key: rec.id ? `id:${rec.id}` : `${rec.rfid}|${date}`,
    id: Number(rec.id) || 0,
    date,
    itemCode: rec.item_code ?? '',
    rfid: rec.rfid ?? '',
    title: rec.title ?? '',
    status: rec.status === 0 ? 'Permitted' : 'Not Permitted',
    imageName: rec.image_filename || null,
  };
}

async function getJSON(url) {
  const res = await fetch(url, { headers: { Accept: 'application/json' } });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function fetchLogs() {
  const json = await getJSON('/rfid/logs');
  if (!json.success) throw new Error(json.message || 'request failed');
  // "no rows" must not be an error: treat null/missing as an empty list
  const rows = json.data == null ? [] : json.data;
  if (!Array.isArray(rows)) throw new Error('Invalid API response: data is not an array');
  return rows.map(mapRecord);
}

// Live scan stream. EventSource retries on its own after a drop; we only step in
// when the browser gave up (readyState CLOSED, e.g. server returned an error).
export function openStream({ onOpen, onEntry, onError }) {
  let es = null;
  let timer = null;
  let stopped = false;

  function connect() {
    es = new EventSource('/rfid/logs/stream');
    es.onopen = () => onOpen();
    es.onmessage = (ev) => {
      try {
        onEntry(mapRecord(JSON.parse(ev.data)));
      } catch (err) {
        console.warn('[sse] malformed payload', ev.data, err);
      }
    };
    es.onerror = () => {
      onError();
      if (es.readyState === EventSource.CLOSED && !stopped) {
        timer = setTimeout(connect, 3000);
      }
    };
  }

  connect();
  return () => {
    stopped = true;
    clearTimeout(timer);
    es?.close();
  };
}
