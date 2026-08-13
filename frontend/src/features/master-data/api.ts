import api from '../../api/client';
import type { Category, CreateCategoryInput, CreateItemConversionInput, CreateItemInput, CreateSupplierInput, CreateSupplierItemInput, CreateUnitInput, Item, ItemConversion, Supplier, SupplierItem, Unit, UpdateCategoryInput, UpdateItemConversionInput, UpdateItemInput, UpdateSupplierInput, UpdateSupplierItemInput, UpdateUnitInput } from './types';

export const unitsApi = {
  list: (includeInactive = true) => api.get<{ units: Unit[] }>('/product/units', { params: { includeInactive } }).then((response) => response.data),
  create: (input: CreateUnitInput) => api.post<{ unit: Unit }>('/product/units', input).then((response) => response.data),
  update: (id: string, input: UpdateUnitInput) => api.patch<{ unit: Unit }>(`/product/units/${id}`, input).then((response) => response.data),
};

export const categoriesApi = {
  list: (includeInactive = true) => api.get<{ categories: Category[] }>('/product/categories', { params: { includeInactive } }).then((response) => response.data),
  create: (input: CreateCategoryInput) => api.post<{ category: Category }>('/product/categories', input).then((response) => response.data),
  update: (id: string, input: UpdateCategoryInput) => api.patch<{ category: Category }>(`/product/categories/${id}`, input).then((response) => response.data),
};

export const itemsApi = {
  list: (includeInactive = true) => api.get<{ items: Item[] }>('/product/items', { params: { includeInactive } }).then((response) => response.data),
  create: (input: CreateItemInput) => api.post<{ item: Item }>('/product/items', input).then((response) => response.data),
  update: (id: string, input: UpdateItemInput) => api.patch<{ item: Item }>(`/product/items/${id}`, input).then((response) => response.data),
};

export const itemConversionsApi = {
  list: (itemId: string) => api.get<{ conversions: ItemConversion[] }>(`/product/items/${itemId}/conversions`).then((response) => response.data),
  create: (itemId: string, input: CreateItemConversionInput) => api.post<{ conversion: ItemConversion }>(`/product/items/${itemId}/conversions`, input).then((response) => response.data),
  update: (itemId: string, conversionId: string, input: UpdateItemConversionInput) => api.patch<{ conversion: ItemConversion }>(`/product/items/${itemId}/conversions/${conversionId}`, input).then((response) => response.data),
};

export const suppliersApi = {
  list: (includeInactive = true) => api.get<{ suppliers: Supplier[] }>('/product/suppliers', { params: { includeInactive } }).then((response) => response.data),
  create: (input: CreateSupplierInput) => api.post<{ supplier: Supplier }>('/product/suppliers', input).then((response) => response.data),
  update: (id: string, input: UpdateSupplierInput) => api.patch<{ supplier: Supplier }>(`/product/suppliers/${id}`, input).then((response) => response.data),
};

export const supplierItemsApi = {
  list: (supplierId: string, includeInactive = true) => api.get<{ supplierItems: SupplierItem[] }>(`/product/suppliers/${supplierId}/items`, { params: { includeInactive } }).then((response) => response.data),
  create: (supplierId: string, input: CreateSupplierItemInput) => api.post<{ supplierItem: SupplierItem }>(`/product/suppliers/${supplierId}/items`, input).then((response) => response.data),
  update: (supplierId: string, supplierItemId: string, input: UpdateSupplierItemInput) => api.patch<{ supplierItem: SupplierItem }>(`/product/suppliers/${supplierId}/items/${supplierItemId}`, input).then((response) => response.data),
};
