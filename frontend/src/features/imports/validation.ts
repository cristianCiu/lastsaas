import type { ImportTarget } from './types';

export const MAX_BYTES = 128 * 1024;
export const MAX_ROWS = 5000;
export function parseCsv(content: string): { headers: string[]; rows: string[][]; delimiter: ',' | ';' } {
  const source = content.replace(/^\uFEFF/, '');
  const delimiter: ',' | ';' = (() => { let commas = 0; let semicolons = 0; let quoted = false; for (const char of source) { if (char === '"') quoted = !quoted; else if (!quoted && char === ',') commas++; else if (!quoted && char === ';') semicolons++; else if (!quoted && char === '\n') break; } return semicolons > commas ? ';' : ','; })();
  const records: string[][] = []; let record: string[] = []; let cell = ''; let quoted = false;
  for (let i = 0; i < source.length; i++) { const char = source[i]; if (char === '"') { if (quoted && source[i + 1] === '"') { cell += '"'; i++; } else quoted = !quoted; } else if (char === delimiter && !quoted) { record.push(cell.trim()); cell = ''; } else if ((char === '\n' || char === '\r') && !quoted) { if (char === '\r' && source[i + 1] === '\n') i++; record.push(cell.trim()); cell = ''; if (record.some((value) => value)) records.push(record); record = []; } else cell += char; }
  record.push(cell.trim()); if (record.some((value) => value)) records.push(record);
  return { headers: records[0] ?? [], rows: records.slice(1), delimiter };
}
export function validateCsv(content: string, target: ImportTarget) {
  const errors: string[] = []; const bytes = new TextEncoder().encode(content).length;
  if (bytes > MAX_BYTES) errors.push('Files must be 128 KiB or smaller.');
  const parsed = parseCsv(content); if (!parsed.headers.length) errors.push('Add a header row.'); if (parsed.rows.length > MAX_ROWS) errors.push('Files can contain at most 5,000 data rows.');
  if (!['units', 'categories', 'items', 'suppliers', 'supplier_items'].includes(target)) errors.push('Choose a supported import target.');
  return errors;
}
