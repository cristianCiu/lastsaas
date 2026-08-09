export const locationKeys = {
  all: ['locations'] as const,
  list: (principalId: string, tenantId: string) => [...locationKeys.all, 'list', principalId, tenantId] as const,
};
