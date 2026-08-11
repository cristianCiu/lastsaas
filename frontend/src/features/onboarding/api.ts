import api from '../../api/client';
import type { RestaurantOnboardingStatus } from './types';

export const onboardingApi = {
  get: () => api.get<{ onboarding: RestaurantOnboardingStatus }>('/product/onboarding').then((response) => response.data),
  complete: () => api.post<{ onboarding: RestaurantOnboardingStatus }>('/product/onboarding/complete').then((response) => response.data),
};
