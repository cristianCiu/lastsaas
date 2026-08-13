import type { ImportTarget } from './types';

export const MAX_BYTES = 128 * 1024;
export const MAX_ROWS = 5000;
export function parseCsv(content: string): { headers: string[]; rows: string[][]; delimiter: ',' | ';' } {
  const lines = content.replace(/^\uFEFF/, '').split(/\r?\n/).filter((line) => line.trim());
  if (!lines.length) return { headers: [], rows: [], delimiter: ',' };
  const delimiter: ',' | ';' = (lines[0].match(/;/g)?.length ?? 0) > (lines[0].match(/,/g)?.length ?? 0) ? ';' : ',';
  const split = (line: string) => line.split(delimiter).map((cell) => cell.trim().replace(/^"|"$/g, ''));
  return { headers: split(lines[0]), rows: lines.slice(1).map(split), delimiter };
}
export function validateCsv(content: string, target: ImportTarget) {
  const errors: string[] = []; const bytes = new TextEncoder().encode(content).length;
  if (bytes > MAX_BYTES) errors.push('Files must be 128 KiB or smaller.');
  const parsed = parseCsv(content); if (!parsed.headers.length) errors.push('Add a header row.'); if (parsed.rows.length > MAX_ROWS) errors.push('Files can contain at most 5,000 data rows.');
  if (!['units', 'categories', 'items', 'suppliers', 'supplier_items'].includes(target)) errors.push('Choose a supported import target.');
  return errors;
}
