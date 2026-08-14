export interface InventoryFormErrors { itemId?: string; storageAreaId?: string; quantity?: string; reason?: string; idempotencyKey?: string; lotId?: string; destinationLocationId?: string; destinationStorageAreaId?: string; }
export function validateInventoryForm(input: { itemId: string; storageAreaId: string; quantity: string; reason: string; idempotencyKey: string; lotId?: string; destinationLocationId?: string; destinationStorageAreaId?: string }, transfer = false, lotRequired = false): InventoryFormErrors {
  const errors: InventoryFormErrors = {};
  if (!input.itemId) errors.itemId = 'Choose an item.';
  if (!input.storageAreaId) errors.storageAreaId = 'Choose a source area.';
  if (!input.quantity || !/^\d+(\.\d+)?$/.test(input.quantity) || Number(input.quantity) <= 0) errors.quantity = 'Enter a positive quantity.';
  if (!input.reason.trim()) errors.reason = 'Add a short reason for this entry.';
  if (lotRequired && !input.lotId) errors.lotId = 'Choose or create a lot for this item.';
  if (input.idempotencyKey.trim().length < 8) errors.idempotencyKey = 'Use at least 8 characters.';
  if (transfer && !input.destinationLocationId) errors.destinationLocationId = 'Choose a destination location.';
  if (transfer && !input.destinationStorageAreaId) errors.destinationStorageAreaId = 'Choose a destination area.';
  return errors;
}
