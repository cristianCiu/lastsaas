export const locationBrandingKeys = {
  all: ['location-branding'] as const,
  detail: (principalId: string, tenantId: string, locationId: string) => [...locationBrandingKeys.all, 'detail', principalId, tenantId, locationId] as const,
};
