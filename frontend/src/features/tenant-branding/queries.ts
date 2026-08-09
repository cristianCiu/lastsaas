export const tenantBrandingKeys = {
  all: ['tenant-branding'] as const,
  detail: (principalId: string, tenantId: string) => [...tenantBrandingKeys.all, 'detail', principalId, tenantId] as const,
  assets: (principalId: string, tenantId: string) => [...tenantBrandingKeys.all, 'assets', principalId, tenantId] as const,
  assetKind: (principalId: string, tenantId: string, kind: string) => [...tenantBrandingKeys.all, 'asset', principalId, tenantId, kind] as const,
  asset: (principalId: string, tenantId: string, kind: string, version: number) => [...tenantBrandingKeys.assetKind(principalId, tenantId, kind), version] as const,
};
