import api from '../../api/client';
import type { ActiveCountListResult, CountLotOptionsResult, InventoryReferences, InventoryWriteInput, JournalEntry, StockBalance, StockLot, TransferInput, LotTrackingMode, StockCountResult, StockCountLineReceipt, ReconciliationRun } from './types';

export const inventoryApi = {
  references: (locationId: string) => api.get<InventoryReferences>(`/product/locations/${locationId}/inventory/references`).then(r => r.data),
  balances: (locationId: string) => api.get<{ balances: StockBalance[] }>(`/product/locations/${locationId}/inventory/balances`).then(r => r.data),
  journal: (locationId: string, itemId?: string) => api.get<{ journal: JournalEntry[] }>(`/product/locations/${locationId}/inventory/journal`, { params: itemId ? { itemId } : undefined }).then(r => r.data),
  adjustment: (locationId: string, input: InventoryWriteInput) => api.post(`/product/locations/${locationId}/inventory/adjustments`, input).then(r => r.data),
  openingBalance: (locationId: string, input: InventoryWriteInput) => api.post(`/product/locations/${locationId}/inventory/opening-balance`, input).then(r => r.data),
  waste: (locationId: string, input: InventoryWriteInput) => api.post(`/product/locations/${locationId}/inventory/waste`, input).then(r => r.data),
  transfer: (locationId: string, input: TransferInput) => api.post(`/product/locations/${locationId}/inventory/transfers`, input).then(r => r.data),
  lots: (itemId: string) => api.get<{ lots: StockLot[] }>(`/product/items/${itemId}/lots`).then(r => r.data),
  createLot: (itemId: string, input: { code: string; expiresAt?: string; receivedAt?: string; status: StockLot['status'] }) => api.post<{ lot: StockLot }>(`/product/items/${itemId}/lots`, input).then(r => r.data),
  updateLot: (itemId: string, lotId: string, status: StockLot['status']) => api.patch(`/product/items/${itemId}/lots/${lotId}`, { status }).then(r => r.data),
  updateTracking: (itemId: string, version: number, mode: LotTrackingMode) => api.patch(`/product/items/${itemId}/lot-tracking`, { version, mode }).then(r => r.data),
  createCount: (locationId: string, storageAreaId: string, idempotencyKey: string) => api.post<StockCountResult>(`/product/locations/${locationId}/inventory/counts`, { storageAreaId, idempotencyKey }).then(r => r.data),
  listActiveCounts: (locationId: string, params?: { cursor?: string; limit?: number }) => api.get<ActiveCountListResult>(`/product/locations/${locationId}/inventory/counts`, { params: { state: 'active', ...params } }).then(r => r.data),
  getCount: (locationId: string, countId: string) => api.get<StockCountResult>(`/product/locations/${locationId}/inventory/counts/${countId}`).then(r => r.data),
  listCountLotOptions: (locationId: string, countId: string, itemId: string) => api.get<CountLotOptionsResult>(`/product/locations/${locationId}/inventory/counts/${countId}/lot-options`, { params: { itemId } }).then(r => r.data),
  freezeCount: (locationId: string, countId: string, version: number) => api.post<StockCountResult>(`/product/locations/${locationId}/inventory/counts/${countId}/freeze`, { version }).then(r => r.data),
  recordCountLine: (locationId: string, countId: string, lineId: string, input: { countVersion: number; lineVersion: number; countedQuantity: string; note?: string }) => api.patch<StockCountLineReceipt>(`/product/locations/${locationId}/inventory/counts/${countId}/lines/${lineId}`, input).then(r => r.data),
  discoverCountLine: (locationId: string, countId: string, input: { countVersion: number; itemId: string; lotId?: string }) => api.post<StockCountLineReceipt>(`/product/locations/${locationId}/inventory/counts/${countId}/lines/discover`, input).then(r => r.data),
  reviewCount: (locationId: string, countId: string, version: number) => api.post<StockCountResult>(`/product/locations/${locationId}/inventory/counts/${countId}/review`, { version }).then(r => r.data),
  cancelCount: (locationId: string, countId: string, version: number) => api.post<StockCountResult>(`/product/locations/${locationId}/inventory/counts/${countId}/cancel`, { version }).then(r => r.data),
  postCount: (locationId: string, countId: string, idempotencyKey: string) => api.post<StockCountResult>(`/product/locations/${locationId}/inventory/counts/${countId}/post`, { idempotencyKey }).then(r => r.data),
  reconcile: (locationId: string, storageAreaId: string | undefined, idempotencyKey: string, repair = false) => api.post<{ run: ReconciliationRun }>(`/product/locations/${locationId}/inventory/reconciliation`, { ...(storageAreaId ? { storageAreaId } : {}), idempotencyKey, repair }).then(r => r.data),
  getReconciliation: (locationId: string, runId: string) => api.get<{ run: ReconciliationRun }>(`/product/locations/${locationId}/inventory/reconciliation/${runId}`).then(r => r.data),
};
