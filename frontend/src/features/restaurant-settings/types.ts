export interface RestaurantSettings {
  currency: string;
  language: string;
  defaultTimezone: string;
  version: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface UpdateRestaurantSettingsInput {
  currency: string;
  language: string;
  defaultTimezone: string;
  version: number;
}
