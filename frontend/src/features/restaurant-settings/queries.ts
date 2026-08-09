export const restaurantSettingsKeys = {
  all: ['restaurant-settings'] as const,
  detail: (tenantId: string) => [...restaurantSettingsKeys.all, 'detail', tenantId] as const,
};
