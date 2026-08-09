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

export type TenantBrandingAssetKind = 'primary' | 'compact';

export interface TenantBrandingAsset {
  id: string;
  kind: TenantBrandingAssetKind;
  contentType: 'image/png' | 'image/jpeg';
  size: number;
  width: number;
  height: number;
  version: number;
  createdAt: string;
  updatedAt: string;
}
