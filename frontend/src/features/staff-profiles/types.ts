export const BUSINESS_ROLES = ['company_owner', 'operations_manager', 'head_chef', 'purchasing', 'stock_service', 'controller', 'viewer'] as const;
export const STORAGE_PERMISSIONS = ['storage_areas.read', 'storage_areas.manage'] as const;

export type BusinessRole = typeof BUSINESS_ROLES[number];
export type StoragePermission = typeof STORAGE_PERMISSIONS[number];
export type StaffProfileStatus = 'active' | 'inactive';

export interface PermissionOverride {
  permission: StoragePermission;
  allowed: boolean;
}

export interface StaffProfile {
  id: string;
  userId: string;
  businessRole: BusinessRole;
  allLocations: boolean;
  locationIds: string[];
  permissionOverrides: PermissionOverride[];
  status: StaffProfileStatus;
  version: number;
  createdAt: string;
  updatedAt: string;
  effectivePermissions: StoragePermission[];
}

export type UpdateStaffProfileInput = Pick<StaffProfile, 'version' | 'businessRole' | 'allLocations' | 'locationIds' | 'permissionOverrides' | 'status'>;
