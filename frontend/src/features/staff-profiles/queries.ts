export const staffProfileKeys = {
  all: ['staff-profiles'] as const,
  self: (principalId: string, tenantId: string) => [...staffProfileKeys.all, 'self', principalId, tenantId] as const,
  list: (principalId: string, tenantId: string) => [...staffProfileKeys.all, 'list', principalId, tenantId] as const,
};
