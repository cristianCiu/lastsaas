export const teamKeys = {
  all: ['team'] as const,
  members: (principalId: string, tenantId: string) => [...teamKeys.all, 'members', principalId, tenantId] as const,
  plans: (principalId: string, tenantId: string) => [...teamKeys.all, 'plans', principalId, tenantId] as const,
};
