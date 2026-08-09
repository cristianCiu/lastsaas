import api from '../../api/client';
import type { TenantBranding, TenantBrandingAsset, TenantBrandingAssetKind, UpdateTenantBrandingInput } from './types';

export const tenantBrandingApi = {
  get: () => api.get<{ branding: TenantBranding }>('/product/branding').then((response) => response.data),
  update: (input: UpdateTenantBrandingInput) =>
    api.put<{ branding: TenantBranding }>('/product/branding', input).then((response) => response.data),
  listAssets: () => api.get<{ assets: TenantBrandingAsset[] }>('/product/branding/assets').then((response) => response.data),
  getAsset: (kind: TenantBrandingAssetKind) => api.get<Blob>(`/product/branding/assets/${kind}`, { responseType: 'blob' }).then((response) => response.data),
  uploadAsset: (kind: TenantBrandingAssetKind, file: File, version: number) => {
    const form = new FormData();
    form.append('file', file);
    form.append('version', String(version));
    return api.put<{ asset: TenantBrandingAsset }>(`/product/branding/assets/${kind}`, form).then((response) => response.data);
  },
  deleteAsset: (kind: TenantBrandingAssetKind, version: number) =>
    api.delete(`/product/branding/assets/${kind}`, { params: { version } }).then(() => undefined),
};
