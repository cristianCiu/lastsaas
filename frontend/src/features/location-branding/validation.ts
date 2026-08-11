import type { BrandingFont } from '../tenant-branding/types';
import { normalizeTenantBranding, validateTenantBranding } from '../tenant-branding/validation';

export interface LocationBrandingFields {
  displayName: string;
  primaryColor: string;
  accentColor: string;
  font: BrandingFont;
}

export type LocationBrandingValidationErrors = Partial<Record<keyof LocationBrandingFields, string>>;

export function validateLocationBranding(fields: LocationBrandingFields): LocationBrandingValidationErrors {
  const errors: LocationBrandingValidationErrors = validateTenantBranding(fields);
  if (fields.displayName.trim().length > 200) errors.displayName = 'Display name must be at most 200 characters.';
  return errors;
}

export function normalizeLocationBranding(fields: LocationBrandingFields): LocationBrandingFields {
  return { displayName: fields.displayName.trim(), ...normalizeTenantBranding(fields) };
}
