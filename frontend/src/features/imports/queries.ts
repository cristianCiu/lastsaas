export const importKeys = {
  all: ['imports'] as const,
  runs: (principalId: string, tenantId: string) => [...importKeys.all, 'runs', principalId, tenantId] as const,
  run: (principalId: string, tenantId: string, id: string) => [...importKeys.all, 'run', principalId, tenantId, id] as const,
};
