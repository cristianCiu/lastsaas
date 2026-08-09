export const locationKeys = {
  all: ['locations'] as const,
  list: (tenantId: string) => [...locationKeys.all, 'list', tenantId] as const,
};
