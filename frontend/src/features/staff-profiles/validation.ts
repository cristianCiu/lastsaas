import { z } from 'zod';
import { BUSINESS_PERMISSIONS, BUSINESS_ROLES } from './types';

const permissionOverrideSchema = z.object({
  permission: z.enum(BUSINESS_PERMISSIONS),
  allowed: z.boolean(),
});
const objectIdSchema = z.string().regex(/^[a-f\d]{24}$/i);

export const staffProfileSchema = z.object({
  id: objectIdSchema,
  userId: objectIdSchema,
  businessRole: z.enum(BUSINESS_ROLES),
  allLocations: z.boolean(),
  locationIds: z.array(z.string()),
  permissionOverrides: z.array(permissionOverrideSchema),
  status: z.enum(['active', 'inactive']),
  version: z.number().int().min(1),
  createdAt: z.iso.datetime({ offset: true }),
  updatedAt: z.iso.datetime({ offset: true }),
  effectivePermissions: z.array(z.enum(BUSINESS_PERMISSIONS)),
});

export const staffProfileResponseSchema = z.object({ staffProfile: staffProfileSchema });
export const staffProfilesResponseSchema = z.object({ staffProfiles: z.array(staffProfileSchema) });
