import type { CreateUnitInput } from './types';
import type { CreateCategoryInput, CreateItemInput } from './types';
import type { CreateSupplierInput, CreateSupplierItemInput } from './types';

export type UnitValidationErrors = Partial<Record<keyof CreateUnitInput, string>>;
const CODE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export function normalizeUnit(input: CreateUnitInput): CreateUnitInput {
  return { ...input, code: input.code.trim().toLowerCase(), name: input.name.trim(), symbol: input.symbol.trim() };
}

export function validateUnit(input: CreateUnitInput): UnitValidationErrors {
  const value = normalizeUnit(input);
  const errors: UnitValidationErrors = {};
  if (!value.code || value.code.length > 32 || !CODE.test(value.code)) errors.code = 'Use a lower-case code up to 32 characters.';
  if (!value.name || value.name.length > 100) errors.name = 'Name is required and must be at most 100 characters.';
  if (!value.symbol || value.symbol.length > 16) errors.symbol = 'Symbol is required and must be at most 16 characters.';
  if (!['mass', 'volume', 'count'].includes(value.dimension)) errors.dimension = 'Choose a supported dimension.';
  if (!Number.isInteger(value.precision) || value.precision < 0 || value.precision > 6) errors.precision = 'Precision must be a whole number from 0 to 6.';
  return errors;
}

export type CategoryValidationErrors = Partial<Record<keyof CreateCategoryInput, string>>;

export function normalizeCategory(input: CreateCategoryInput): CreateCategoryInput {
  return { code: input.code.trim().toLowerCase(), name: input.name.trim() };
}

export function validateCategory(input: CreateCategoryInput): CategoryValidationErrors {
  const value = normalizeCategory(input);
  const errors: CategoryValidationErrors = {};
  if (!value.code || value.code.length > 32 || !CODE.test(value.code)) errors.code = 'Use a lower-case code up to 32 characters.';
  if (!value.name || value.name.length > 100) errors.name = 'Name is required and must be at most 100 characters.';
  return errors;
}

export type ItemValidationErrors = Partial<Record<keyof CreateItemInput, string>>;

export function normalizeItem(input: CreateItemInput): CreateItemInput {
  return { ...input, sku: input.sku.trim().toLowerCase(), name: input.name.trim(), description: input.description?.trim() || undefined, brand: input.brand?.trim() || undefined };
}

export function validateItem(input: CreateItemInput): ItemValidationErrors {
  const value = normalizeItem(input); const errors: ItemValidationErrors = {};
  if (!value.sku || value.sku.length > 64 || !CODE.test(value.sku)) errors.sku = 'Use a lower-case SKU up to 64 characters.';
  if (!value.name || value.name.length > 160) errors.name = 'Name is required and must be at most 160 characters.';
  if (value.description && value.description.length > 2000) errors.description = 'Description must be at most 2000 characters.';
  if (value.brand && value.brand.length > 100) errors.brand = 'Brand must be at most 100 characters.';
  if (!value.categoryId) errors.categoryId = 'Choose a category.';
  if (!value.baseUnitId) errors.baseUnitId = 'Choose a base unit.';
  if (value.shelfLifeDays !== undefined && (!Number.isInteger(value.shelfLifeDays) || value.shelfLifeDays < 0 || value.shelfLifeDays > 36500)) errors.shelfLifeDays = 'Shelf life must be a whole number from 0 to 36500.';
  if (typeof value.stockable !== 'boolean') errors.stockable = 'Choose whether this item is stockable.';
  return errors;
}

export type SupplierValidationErrors = Partial<Record<keyof CreateSupplierInput, string>>;
export function normalizeSupplier(input: CreateSupplierInput): CreateSupplierInput { return { ...input, code: input.code.trim().toLowerCase(), name: input.name.trim(), contactName: input.contactName?.trim() || undefined, email: input.email?.trim() || undefined, phone: input.phone?.trim() || undefined }; }
export function validateSupplier(input: CreateSupplierInput): SupplierValidationErrors { const value = normalizeSupplier(input); const errors: SupplierValidationErrors = {}; if (!value.code || value.code.length > 64 || !CODE.test(value.code)) errors.code = 'Use a lower-case code up to 64 characters.'; if (!value.name || value.name.length > 160) errors.name = 'Name is required and must be at most 160 characters.'; if (value.email && !/^\S+@\S+\.\S+$/.test(value.email)) errors.email = 'Enter a valid email address.'; if (value.orderingDays.some((day) => !Number.isInteger(day) || day < 1 || day > 7)) errors.orderingDays = 'Choose valid weekdays.'; if (!Number.isInteger(value.defaultLeadTimeDays) || value.defaultLeadTimeDays < 0) errors.defaultLeadTimeDays = 'Lead time must be a whole number.'; return errors; }
export type SupplierItemValidationErrors = Partial<Record<keyof CreateSupplierItemInput, string>>;
export function validateSupplierItem(input: CreateSupplierItemInput): SupplierItemValidationErrors { const errors: SupplierItemValidationErrors = {}; const int64 = /^(0|[1-9]\d*)$/; const positiveInt64 = /^[1-9]\d*$/; if (!input.itemId) errors.itemId = 'Choose an item.'; if (!positiveInt64.test(input.packSizeMicros)) errors.packSizeMicros = 'Use a positive whole-number micro-unit value.'; if (!Number.isInteger(input.moq) || input.moq < 1 || input.moq > 1000000) errors.moq = 'Use a whole number from 1 to 1,000,000.'; if (!int64.test(input.unitPriceMinor)) errors.unitPriceMinor = 'Use non-negative minor currency units.'; if (!/^[A-Z]{3}$/.test(input.currency)) errors.currency = 'Use a 3-letter currency code.'; if (input.leadTimeDays !== null && input.leadTimeDays !== undefined && (!Number.isInteger(input.leadTimeDays) || input.leadTimeDays < 0)) errors.leadTimeDays = 'Lead time must be a whole number.'; return errors; }
