import api from '../../api/client';
import type { CreateLocationInput, Location, UpdateLocationInput } from './types';

export const locationsApi = {
  list: () =>
    api.get<{ locations: Location[] }>('/product/locations').then((response) => response.data),
  create: (input: CreateLocationInput) =>
    api.post<{ location: Location }>('/product/locations', input).then((response) => response.data),
  update: (id: string, input: UpdateLocationInput) =>
    api.patch<{ location: Location }>(`/product/locations/${id}`, input).then((response) => response.data),
};
