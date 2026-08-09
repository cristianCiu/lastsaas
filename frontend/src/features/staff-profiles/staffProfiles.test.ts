import { describe, expect, it } from 'vitest';
import { isMissingStaffProfileError } from '../../contexts/StaffProfileContext';
import { hasStaffPermission } from './capabilities';
import { staffProfileKeys } from './queries';
import type { StaffProfile } from './types';
import { staffProfileResponseSchema } from './validation';

const profile: StaffProfile = {
  id: '65a000000000000000000001', userId: '65a000000000000000000002', businessRole: 'head_chef', allLocations: false, locationIds: [], permissionOverrides: [], status: 'active', version: 2,
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-02T00:00:00Z',
  effectivePermissions: ['storage_areas.read'],
};

describe('staff capabilities', () => {
  it('uses effective permissions only for active profiles', () => {
    expect(hasStaffPermission(profile, 'storage_areas.read')).toBe(true);
    expect(hasStaffPermission(profile, 'storage_areas.manage')).toBe(false);
    expect(hasStaffPermission({ ...profile, status: 'inactive' }, 'storage_areas.read')).toBe(false);
    expect(hasStaffPermission(null, 'storage_areas.read')).toBe(false);
  });
});

describe('staff profile contract', () => {
  it('parses active and inactive backend response shapes', () => {
    expect(staffProfileResponseSchema.parse({ staffProfile: profile }).staffProfile).toEqual(profile);
    expect(staffProfileResponseSchema.parse({ staffProfile: { ...profile, status: 'inactive', effectivePermissions: [] } }).staffProfile.status).toBe('inactive');
  });

  it('rejects malformed backend fields and unknown permissions', () => {
    expect(() => staffProfileResponseSchema.parse({ staffProfile: { ...profile, permissionOverrides: [{ permission: 'orders.manage', allowed: true }] } })).toThrow();
    expect(() => staffProfileResponseSchema.parse({ staffProfile: { ...profile, version: 0 } })).toThrow();
    expect(() => staffProfileResponseSchema.parse({ staffProfile: { ...profile, effectivePermissions: undefined } })).toThrow();
  });

  it('recognizes the real structured missing-profile error shape', () => {
    expect(isMissingStaffProfileError({ isAxiosError: true, response: { status: 404, data: { error: 'Staff profile not found', code: 'NOT_FOUND', requestId: 'req-1' } } })).toBe(true);
    expect(isMissingStaffProfileError({ isAxiosError: true, response: { status: 404, data: { error: 'Wrong tenant', code: 'FORBIDDEN' } } })).toBe(false);
    expect(isMissingStaffProfileError({ isAxiosError: true, response: { status: 403, data: { error: 'Inactive', code: 'STAFF_PROFILE_INACTIVE' } } })).toBe(false);
  });
});

describe('staff profile query keys', () => {
  it('isolates self and list data by principal and tenant', () => {
    expect(staffProfileKeys.self('user-a', 'tenant-a')).toEqual(['staff-profiles', 'self', 'user-a', 'tenant-a']);
    expect(staffProfileKeys.list('user-a', 'tenant-a')).toEqual(['staff-profiles', 'list', 'user-a', 'tenant-a']);
    expect(staffProfileKeys.self('user-a', 'tenant-a')).not.toEqual(staffProfileKeys.self('user-b', 'tenant-a'));
  });
});
