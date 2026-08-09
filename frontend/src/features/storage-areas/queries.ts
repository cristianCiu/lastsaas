export const storageAreaKeys = {
  all: ['storage-areas'] as const,
  list: (principalId: string, tenantId: string, locationId: string) => [...storageAreaKeys.all, 'list', principalId, tenantId, locationId] as const,
};
