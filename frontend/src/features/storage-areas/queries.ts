export const storageAreaKeys = {
  all: ['storage-areas'] as const,
  list: (tenantId: string, locationId: string) => [...storageAreaKeys.all, 'list', tenantId, locationId] as const,
};
