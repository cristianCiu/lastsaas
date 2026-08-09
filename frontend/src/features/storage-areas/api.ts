import api from '../../api/client';
import type { CreateStorageAreaInput, StorageArea, UpdateStorageAreaInput } from './types';

export const storageAreasApi = {
  list: (locationId: string) => api.get<{ storageAreas: StorageArea[] }>(`/product/locations/${locationId}/storage-areas`).then((response) => response.data),
  create: (locationId: string, input: CreateStorageAreaInput) =>
    api.post<{ storageArea: StorageArea }>(`/product/locations/${locationId}/storage-areas`, input).then((response) => response.data),
  update: (locationId: string, id: string, input: UpdateStorageAreaInput) =>
    api.patch<{ storageArea: StorageArea }>(`/product/locations/${locationId}/storage-areas/${id}`, input).then((response) => response.data),
};
