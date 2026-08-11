import api from '../../api/client';
import type { LocationBrandingResponse, UpdateLocationBrandingInput } from './types';

export const locationBrandingApi = {
  get: (locationId: string) => api.get<LocationBrandingResponse>(`/product/locations/${locationId}/branding`).then((response) => response.data),
  update: (locationId: string, input: UpdateLocationBrandingInput) =>
    api.put<Pick<LocationBrandingResponse, 'branding'>>(`/product/locations/${locationId}/branding`, input).then((response) => response.data),
  reset: (locationId: string, version: number) =>
    api.delete(`/product/locations/${locationId}/branding`, { params: { version } }).then(() => undefined),
};
