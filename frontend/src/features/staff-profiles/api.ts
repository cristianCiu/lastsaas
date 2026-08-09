import api from '../../api/client';
import type { UpdateStaffProfileInput } from './types';
import { staffProfileResponseSchema, staffProfilesResponseSchema } from './validation';

export const staffProfilesApi = {
  getSelf: () => api.get('/product/staff-profile').then((response) => staffProfileResponseSchema.parse(response.data)),
  list: () => api.get('/product/staff-profiles').then((response) => staffProfilesResponseSchema.parse(response.data)),
  update: (userId: string, input: UpdateStaffProfileInput) =>
    api.put(`/product/staff-profiles/${userId}`, input).then((response) => staffProfileResponseSchema.parse(response.data)),
};
