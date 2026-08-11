export const BUSINESS_ROLES = ['company_owner', 'operations_manager', 'head_chef', 'purchasing', 'stock_service', 'controller', 'viewer'] as const;
export const BUSINESS_PERMISSIONS = ['storage_areas.read', 'storage_areas.manage', 'catalog.read', 'catalog.manage'] as const;

export type BusinessRole = typeof BUSINESS_ROLES[number];
export type BusinessPermission = typeof BUSINESS_PERMISSIONS[number];
export type StaffProfileStatus = 'active' | 'inactive';

export interface PermissionOverride {
  permission: BusinessPermission;
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
  effectivePermissions: BusinessPermission[];
}

export type UpdateStaffProfileInput = Pick<StaffProfile, 'version' | 'businessRole' | 'allLocations' | 'locationIds' | 'permissionOverrides' | 'status'>;
