export const onboardingKeys = {
  all: ['restaurant-onboarding'] as const,
  detail: (principalId: string, tenantId: string) => [...onboardingKeys.all, 'detail', principalId, tenantId] as const,
};
