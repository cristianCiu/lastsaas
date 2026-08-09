import { isValidTimezone } from '../locations/validation';
import type { UpdateRestaurantSettingsInput } from './types';

export type RestaurantSettingsFields = Omit<UpdateRestaurantSettingsInput, 'version'>;
export type RestaurantSettingsValidationErrors = Partial<Record<keyof RestaurantSettingsFields, string>>;

const CURRENCY_PATTERN = /^[A-Z]{3}$/;
const LANGUAGE_PATTERN = /^[a-z]{2}(?:-[A-Z]{2})?$/;

export function normalizeLanguage(language: string): string {
  const parts = language.trim().replaceAll('_', '-').split('-');
  return parts.map((part, index) => index === 0 ? part.toLowerCase() : part.toUpperCase()).join('-');
}

export function normalizeRestaurantSettings(input: RestaurantSettingsFields): RestaurantSettingsFields {
  return {
    currency: input.currency.trim().toUpperCase(),
    language: normalizeLanguage(input.language),
    defaultTimezone: input.defaultTimezone.trim(),
  };
}

export function validateRestaurantSettings(input: RestaurantSettingsFields): RestaurantSettingsValidationErrors {
  const normalized = normalizeRestaurantSettings(input);
  const errors: RestaurantSettingsValidationErrors = {};

  if (!CURRENCY_PATTERN.test(normalized.currency)) {
    errors.currency = 'Enter a three-letter currency code, such as EUR.';
  }
  if (!LANGUAGE_PATTERN.test(normalized.language)) {
    errors.language = 'Enter a practical BCP-47 language tag, such as de or en-US.';
  }
  if (!normalized.defaultTimezone) {
    errors.defaultTimezone = 'Default timezone is required.';
  } else if (!isValidTimezone(normalized.defaultTimezone)) {
    errors.defaultTimezone = 'Enter a valid IANA timezone, such as Europe/Berlin.';
  }

  return errors;
}
