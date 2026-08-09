import type { CreateLocationInput } from './types';

export type LocationValidationErrors = Partial<Record<keyof CreateLocationInput, string>>;

const CODE_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export function isValidTimezone(timezone: string): boolean {
  if (!timezone.trim()) return false;

  try {
    new Intl.DateTimeFormat('en-US', { timeZone: timezone }).format();
    return true;
  } catch {
    return false;
  }
}

export function validateLocation(input: CreateLocationInput): LocationValidationErrors {
  const errors: LocationValidationErrors = {};

  if (!input.code.trim()) {
    errors.code = 'Code is required.';
  } else if (input.code.trim().length > 64) {
    errors.code = 'Code must be 64 characters or fewer.';
  } else if (!CODE_PATTERN.test(input.code.trim())) {
    errors.code = 'Use a lower-case slug with letters, numbers, and single hyphens.';
  }

  if (!input.name.trim()) {
    errors.name = 'Name is required.';
  } else if (input.name.trim().length > 200) {
    errors.name = 'Name must be 200 characters or fewer.';
  }

  if (!input.timezone.trim()) {
    errors.timezone = 'Timezone is required.';
  } else if (input.timezone.trim().length > 100) {
    errors.timezone = 'Timezone must be 100 characters or fewer.';
  } else if (!isValidTimezone(input.timezone.trim())) {
    errors.timezone = 'Enter a valid IANA timezone, such as America/New_York.';
  }

  return errors;
}

export function getTimezoneOptions(): string[] {
  try {
    const intl = Intl as typeof Intl & { supportedValuesOf?: (key: 'timeZone') => string[] };
    const supported = intl.supportedValuesOf?.('timeZone');
    if (supported?.length) return supported;
  } catch {
    // Older browsers can still use and validate a manually entered timezone.
  }

  return [
    'America/Chicago',
    'America/Denver',
    'America/Los_Angeles',
    'America/New_York',
    'Asia/Singapore',
    'Asia/Tokyo',
    'Australia/Sydney',
    'Europe/Berlin',
    'Europe/London',
    'Pacific/Auckland',
  ];
}
