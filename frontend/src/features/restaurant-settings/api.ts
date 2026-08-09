import api from '../../api/client';
import type { RestaurantSettings, UpdateRestaurantSettingsInput } from './types';

export const restaurantSettingsApi = {
  get: () => api.get<{ settings: RestaurantSettings }>('/product/restaurant-settings').then((response) => response.data),
  update: (input: UpdateRestaurantSettingsInput) =>
    api.put<{ settings: RestaurantSettings }>('/product/restaurant-settings', input).then((response) => response.data),
};
