export function cleanVoiceName(rawName: string, id: string): string {
  if (!rawName) return id;
  return rawName
    .replace(/^Microsoft\s+/i, '')
    .replace(/\s+Online\s+\(Natural\)\s*-\s*/i, ' - ')
    .replace(/\s+Online\s+\(Natural\)/i, '')
    .trim();
}

export function normalizeLangCode(raw: string): string {
  if (!raw) return 'es';
  const lower = raw.trim().toLowerCase().replace('_', '-');
  if (lower === 'all' || lower === '*') return 'all';
  const base = lower.split('-')[0];
  const iso3Map: Record<string, string> = {
    spa: 'es',
    eng: 'en',
    fra: 'fr',
    fre: 'fr',
    deu: 'de',
    ger: 'de',
    ita: 'it',
    por: 'pt',
    cat: 'ca',
    glg: 'gl',
    eus: 'eu',
    baq: 'eu',
    nld: 'nl',
    dut: 'nl',
    pol: 'pl',
    rus: 'ru',
    jpn: 'ja',
    zho: 'zh',
    chi: 'zh',
  };
  return iso3Map[base] || base;
}

export function parseChapterCount(input: string, total: number): number {
  const trimmed = input.trim().toLowerCase();
  if (!trimmed || trimmed === 'all' || trimmed === '*') {
    return total;
  }
  const seen = new Set<number>();
  const parts = trimmed.split(',');
  for (const part of parts) {
    const p = part.trim();
    if (!p) continue;
    if (p.includes('-')) {
      const [startStr, endStr] = p.split('-');
      const start = parseInt(startStr, 10);
      const end = parseInt(endStr, 10);
      if (!isNaN(start) && !isNaN(end)) {
        const [low, high] = start <= end ? [start, end] : [end, start];
        for (let i = low; i <= high; i++) {
          if (i >= 0 && i < total) seen.add(i);
        }
      }
    } else {
      const idx = parseInt(p, 10);
      if (!isNaN(idx) && idx >= 0 && idx < total) {
        seen.add(idx);
      }
    }
  }
  return seen.size || total;
}
