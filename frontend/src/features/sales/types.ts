export const SALES_FIELDS = ['source', 'externalSaleId', 'externalLineId', 'externalProductId', 'quantity', 'occurredAt', 'locationId', 'storageAreaId', 'status'] as const;
export type SalesField = typeof SALES_FIELDS[number];
export interface SalesImportError { row: number; field?: string; code: string; message: string }
export interface SalesImportRun { id: string; source: string; status: 'pending' | 'completed' | 'failed'; totalRows: number; createdSales: number; createdLines: number; unresolvedRows: number; errors: SalesImportError[]; createdAt: string; updatedAt: string }
export interface SalesImportReport { run?: SalesImportRun; errors: SalesImportError[]; dryRun: boolean; totalRows: number; createdSales: number; createdLines: number; unresolvedRows: number }
export interface UnmappedSaleLine { id: string; importRunId: string; source: string; externalSaleId: string; externalLineId: string; externalProductId: string; occurredAt: string; locationId: string; storageAreaId: string; quantityMicros: string; reason: string; createdAt: string }
