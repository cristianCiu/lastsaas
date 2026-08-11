import type { BrandingFont } from '../tenant-branding/types';

export interface LocationBranding {
  id?: string;
  displayName: string;
  primaryColor: string;
  accentColor: string;
  font: BrandingFont;
  version: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface ResolvedLocationBranding {
  locationId: string;
  displayName: string;
  primaryColor: string;
  accentColor: string;
  font: BrandingFont;
  locationBrandingVersion: number;
  tenantBrandingVersion: number;
  sources: Record<'displayName' | 'primaryColor' | 'accentColor' | 'font', 'location' | 'location_branding' | 'tenant' | 'platform'>;
}

export interface LocationBrandingResponse {
  branding: LocationBranding;
  resolved: ResolvedLocationBranding;
  entitled: boolean;
}

export interface UpdateLocationBrandingInput {
  displayName: string;
  primaryColor: string;
  accentColor: string;
  font: BrandingFont;
  version: number;
}
