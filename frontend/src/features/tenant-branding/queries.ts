export const tenantBrandingKeys = {
  all: ['tenant-branding'] as const,
  detail: (principalId: string, tenantId: string) => [...tenantBrandingKeys.all, 'detail', principalId, tenantId] as const,
};
