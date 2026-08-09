import type { StaffProfile } from './types';

export function hasStaffPermission(profile: StaffProfile | null | undefined, permission: string): boolean {
  return profile?.status === 'active' && profile.effectivePermissions.some((effective) => effective === permission);
}
