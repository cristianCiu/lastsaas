import api from '../../api/client';
import type { CreateUnitInput, Unit, UpdateUnitInput } from './types';

export const unitsApi = {
  list: (includeInactive = true) => api.get<{ units: Unit[] }>('/product/units', { params: { includeInactive } }).then((response) => response.data),
  create: (input: CreateUnitInput) => api.post<{ unit: Unit }>('/product/units', input).then((response) => response.data),
  update: (id: string, input: UpdateUnitInput) => api.patch<{ unit: Unit }>(`/product/units/${id}`, input).then((response) => response.data),
};
