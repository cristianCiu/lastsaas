import type { Location } from '../locations/types';

export type LotStatus = 'available' | 'quarantined';
export type LotTrackingMode = '' | 'optional' | 'required';
export type PostingType = 'opening_balance' | 'manual_adjustment' | 'reversal' | 'transfer' | 'waste' | 'stock_count';

export interface StockBalance { id: string; locationId: string; storageAreaId: string; itemId: string; lotId?: string; quantityMicros: string; updatedAt: string; }
export interface StockPosting { id: string; locationId: string; storageAreaId: string; type: PostingType; effectiveAt: string; recordedAt: string; reason?: string; destinationLocationId?: string; destinationStorageAreaId?: string; }
export interface JournalEntry { posting: StockPosting; movement: { itemId: string; storageAreaId: string; lotId?: string; quantityMicros: string }; }
export interface StockLot { id: string; itemId: string; code: string; expiresAt?: string; receivedAt: string; status: LotStatus; }
export interface InventoryWriteInput { storageAreaId: string; itemId: string; quantity: string; idempotencyKey: string; lotId?: string; reason?: string; }
export interface TransferInput extends InventoryWriteInput { destinationLocationId: string; destinationStorageAreaId: string; }
export interface InventoryItemReference { id: string; sku: string; name: string; lotTracking?: LotTrackingMode; version?: number; }
export interface InventoryStorageAreaReference { id: string; locationId: string; name: string; type: string; }
export interface InventoryReferences { items: InventoryItemReference[]; storageAreas: InventoryStorageAreaReference[]; }
export interface InventoryLookups { items: InventoryItemReference[]; locations: Location[]; areas: InventoryStorageAreaReference[]; }
export type StockCountStatus = 'draft' | 'frozen' | 'reviewed' | 'posted' | 'cancelled';
export interface StockCount { id: string; locationId: string; storageAreaId: string; status: StockCountStatus; version: number; cutoffAt?: string; createdAt: string; updatedAt: string; frozenAt?: string; reviewedAt?: string; postedAt?: string; cancelledAt?: string; }
export interface StockCountLine { id: string; countId: string; locationId: string; storageAreaId: string; itemId: string; lotId?: string; lotCode?: string; lotStatus?: LotStatus; lotExpiresAt?: string; snapshotQuantityMicros: string; countedQuantityMicros?: string; version: number; note?: string; }
export interface StockCountResult { count: StockCount; lines: StockCountLine[]; posting?: StockPosting; }
export interface StockCountLineReceipt { line: StockCountLine; }
export type ActiveCountIntegrity = 'healthy' | 'owner_missing' | 'owner_mismatch' | 'area_missing';
export interface ActiveCountSummary { count: StockCount; integrity: ActiveCountIntegrity; }
export interface ActiveCountListResult { counts: ActiveCountSummary[]; nextCursor?: string; }
export interface CountLotOption { id: string; code: string; status: LotStatus; expiresAt?: string; }
export interface CountLotOptionsResult { lots: CountLotOption[]; }
export interface ReconciliationMismatch { locationId: string; storageAreaId: string; itemId: string; lotId?: string; expectedQuantityMicros: string; actualQuantityMicros: string; deltaMicros: string; }
export interface ReconciliationRun { id: string; locationId: string; storageAreaId?: string; cutoffAt: string; status: 'detected' | 'repaired'; mismatchCount: number; truncated: boolean; mismatches: ReconciliationMismatch[]; createdAt: string; completedAt: string; }
