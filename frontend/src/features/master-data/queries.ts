export const masterDataKeys = {
  all: ['master-data'] as const,
  units: (principalId: string, tenantId: string) => [...masterDataKeys.all, 'units', principalId, tenantId] as const,
  categories: (principalId: string, tenantId: string) => [...masterDataKeys.all, 'categories', principalId, tenantId] as const,
  items: (principalId: string, tenantId: string) => [...masterDataKeys.all, 'items', principalId, tenantId] as const,
  itemConversions: (principalId: string, tenantId: string, itemId: string) => [...masterDataKeys.all, 'item-conversions', principalId, tenantId, itemId] as const,
  suppliers: (principalId: string, tenantId: string) => [...masterDataKeys.all, 'suppliers', principalId, tenantId] as const,
  supplierItems: (principalId: string, tenantId: string, supplierId: string) => [...masterDataKeys.all, 'supplier-items', principalId, tenantId, supplierId] as const,
};
