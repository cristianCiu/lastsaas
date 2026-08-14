export const inventoryKeys = {
  all: ['inventory'] as const,
  references: (principal: string, tenant: string, location: string) => [...inventoryKeys.all, 'references', principal, tenant, location] as const,
  balances: (principal: string, tenant: string, location: string) => [...inventoryKeys.all, 'balances', principal, tenant, location] as const,
  journal: (principal: string, tenant: string, location: string, item = '') => [...inventoryKeys.all, 'journal', principal, tenant, location, item] as const,
  lots: (principal: string, tenant: string, item: string) => [...inventoryKeys.all, 'lots', principal, tenant, item] as const,
  activeCounts: (principal: string, tenant: string, location: string, cursor = '', limit = 0) => [...inventoryKeys.all, 'active-counts', principal, tenant, location, cursor, limit] as const,
  count: (principal: string, tenant: string, location: string, countId: string) => [...inventoryKeys.all, 'count', principal, tenant, location, countId] as const,
  countLotOptions: (principal: string, tenant: string, location: string, countId: string, itemId: string) => [...inventoryKeys.all, 'count-lot-options', principal, tenant, location, countId, itemId] as const,
  reconciliation: (principal: string, tenant: string, location: string, runId: string) => [...inventoryKeys.all, 'reconciliation', principal, tenant, location, runId] as const,
};
