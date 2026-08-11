export const masterDataKeys = {
  all: ['master-data'] as const,
  units: (principalId: string, tenantId: string) => [...masterDataKeys.all, 'units', principalId, tenantId] as const,
};
