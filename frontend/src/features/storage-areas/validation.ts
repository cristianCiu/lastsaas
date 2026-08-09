import { STORAGE_AREA_TYPES, type CreateStorageAreaInput, type StorageAreaType } from './types';

export type StorageAreaValidationErrors = Partial<Record<keyof CreateStorageAreaInput, string>>;

export function isStorageAreaType(value: string): value is StorageAreaType {
  return STORAGE_AREA_TYPES.some((type) => type === value);
}

export function validateStorageArea(input: { name: string; type: string }): StorageAreaValidationErrors {
  const errors: StorageAreaValidationErrors = {};
  const name = input.name.trim();

  if (!name) errors.name = 'Name is required.';
  else if (name.length > 200) errors.name = 'Name must be 200 characters or fewer.';
  if (!isStorageAreaType(input.type)) errors.type = 'Choose a valid storage area type.';

  return errors;
}
