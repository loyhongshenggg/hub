const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
];

const rtf = new Intl.RelativeTimeFormat(undefined, {
  numeric: 'auto',
  style: 'short',
});

/** "3 hr. ago", "yesterday", "just now". */
export function ago(iso: string | null | undefined, now = Date.now()): string {
  if (!iso) return 'never';
  const secs = (new Date(iso).getTime() - now) / 1000;
  for (const [unit, size] of units) {
    if (Math.abs(secs) >= size)
      return rtf.format(Math.round(secs / size), unit);
  }
  return 'just now';
}

export function host(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, '');
  } catch {
    return url;
  }
}
