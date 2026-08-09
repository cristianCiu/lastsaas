import type { BrandingFont, UpdateTenantBrandingInput } from './types';

export type TenantBrandingFields = Omit<UpdateTenantBrandingInput, 'version'>;
export type TenantBrandingValidationErrors = Partial<Record<keyof TenantBrandingFields, string>>;

export const BRANDING_FONTS: ReadonlyArray<{ value: BrandingFont; label: string; stack: string }> = [
  { value: '', label: 'Platform default', stack: '' },
  { value: 'system', label: 'System sans', stack: 'system-ui, -apple-system, sans-serif' },
  { value: 'humanist', label: 'Humanist sans', stack: '"Trebuchet MS", "Segoe UI", sans-serif' },
  { value: 'geometric', label: 'Geometric sans', stack: 'Avenir, Montserrat, "Century Gothic", sans-serif' },
  { value: 'serif', label: 'Editorial serif', stack: 'Georgia, "Times New Roman", serif' },
];

const HEX_COLOR = /^#[0-9a-f]{6}$/;

export function normalizeTenantBranding(input: TenantBrandingFields): TenantBrandingFields {
  return {
    primaryColor: input.primaryColor.trim().toLowerCase(),
    accentColor: input.accentColor.trim().toLowerCase(),
    font: input.font,
  };
}

export function validateTenantBranding(input: TenantBrandingFields): TenantBrandingValidationErrors {
  const normalized = normalizeTenantBranding(input);
  const errors: TenantBrandingValidationErrors = {};
  if (normalized.primaryColor && !HEX_COLOR.test(normalized.primaryColor)) {
    errors.primaryColor = 'Use a six-digit hex color, such as #0ea5e9.';
  }
  if (normalized.accentColor && !HEX_COLOR.test(normalized.accentColor)) {
    errors.accentColor = 'Use a six-digit hex color, such as #a855f7.';
  }
  if (!BRANDING_FONTS.some((font) => font.value === normalized.font)) {
    errors.font = 'Choose one of the supported font styles.';
  }
  return errors;
}

export function getBrandingFontStack(font: BrandingFont): string {
  return BRANDING_FONTS.find((option) => option.value === font)?.stack ?? '';
}
