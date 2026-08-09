export type BrandingFont = '' | 'system' | 'humanist' | 'geometric' | 'serif';

export interface TenantBranding {
  primaryColor: string;
  accentColor: string;
  font: BrandingFont;
  version: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface UpdateTenantBrandingInput {
  primaryColor: string;
  accentColor: string;
  font: BrandingFont;
  version: number;
}
