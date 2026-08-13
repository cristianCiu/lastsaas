export type UnitDimension = 'mass' | 'volume' | 'count';

export interface Unit {
  id: string;
  code: string;
  name: string;
  symbol: string;
  dimension: UnitDimension;
  precision: number;
  isActive: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateUnitInput {
  code: string;
  name: string;
  symbol: string;
  dimension: UnitDimension;
  precision: number;
}

export interface UpdateUnitInput {
  version: number;
  name?: string;
  symbol?: string;
  precision?: number;
  isActive?: boolean;
}

export interface Category {
  id: string;
  code: string;
  name: string;
  isActive: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateCategoryInput {
  code: string;
  name: string;
}

export interface UpdateCategoryInput {
  version: number;
  code?: string;
  name?: string;
  isActive?: boolean;
}

export const ALLERGEN_CODES = ['celery', 'cereals-gluten', 'crustaceans', 'eggs', 'fish', 'lupin', 'milk', 'molluscs', 'mustard', 'nuts', 'peanuts', 'sesame', 'soy', 'sulphites'] as const;
export type AllergenCode = typeof ALLERGEN_CODES[number];

export interface Item {
  id: string;
  sku: string;
  name: string;
  description?: string;
  brand?: string;
  categoryId: string;
  baseUnitId: string;
  allergens: AllergenCode[];
  shelfLifeDays?: number;
  stockable: boolean;
  isActive: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateItemInput {
  sku: string;
  name: string;
  description?: string;
  brand?: string;
  categoryId: string;
  baseUnitId: string;
  allergens: AllergenCode[];
  shelfLifeDays?: number;
  stockable: boolean;
}

export interface UpdateItemInput extends Partial<Omit<CreateItemInput, 'sku'>> {
  version: number;
  isActive?: boolean;
}

export interface ItemConversion {
  id: string;
  itemId: string;
  fromUnitId: string;
  numerator: string;
  denominator: string;
  isActive: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateItemConversionInput {
  fromUnitId: string;
  numerator: string;
  denominator: string;
}

export interface UpdateItemConversionInput extends Partial<CreateItemConversionInput> {
  version: number;
  isActive?: boolean;
}

export interface Supplier { id: string; code: string; name: string; contactName?: string; email?: string; phone?: string; orderingDays: number[]; defaultLeadTimeDays: number; isActive: boolean; version: number; createdAt: string; updatedAt: string; }
export interface CreateSupplierInput { code: string; name: string; contactName?: string; email?: string; phone?: string; orderingDays: number[]; defaultLeadTimeDays: number; }
export interface UpdateSupplierInput extends Partial<CreateSupplierInput> { version: number; isActive?: boolean; }
export interface SupplierItem { id: string; supplierId: string; itemId: string; supplierSKU?: string; packSizeMicros: string; moq: number; unitPriceMinor: string; currency: string; leadTimeDays?: number | null; isActive: boolean; version: number; createdAt: string; updatedAt: string; }
export interface CreateSupplierItemInput { itemId: string; supplierSKU?: string; packSizeMicros: string; moq: number; unitPriceMinor: string; currency: string; leadTimeDays?: number | null; }
export interface UpdateSupplierItemInput extends Partial<Omit<CreateSupplierItemInput, 'itemId'>> { version: number; isActive?: boolean; }
