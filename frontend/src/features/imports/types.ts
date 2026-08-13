export const IMPORT_TARGETS = ['units', 'categories', 'items', 'suppliers', 'supplier_items'] as const;
export type ImportTarget = typeof IMPORT_TARGETS[number];
export type ImportMapping = Record<string, string>;
export interface ImportError { row: number; field?: string; code: string; message: string; }
export interface ImportReport { run?: ImportRun; errors: ImportError[]; dryRun: boolean; totalRows: number; createdRows: number; updatedRows: number; }
export interface ImportRun { id: string; target: ImportTarget; status: string; totalRows: number; createdRows: number; updatedRows: number; errors: ImportError[]; createdAt: string; updatedAt: string; }
export interface ImportRequest { target: ImportTarget; content: string; mapping?: ImportMapping; idempotencyKey: string; }
