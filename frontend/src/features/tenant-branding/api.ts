import api from '../../api/client';
import type { TenantBranding, UpdateTenantBrandingInput } from './types';

export const tenantBrandingApi = {
  get: () => api.get<{ branding: TenantBranding }>('/product/branding').then((response) => response.data),
  update: (input: UpdateTenantBrandingInput) =>
    api.put<{ branding: TenantBranding }>('/product/branding', input).then((response) => response.data),
};
