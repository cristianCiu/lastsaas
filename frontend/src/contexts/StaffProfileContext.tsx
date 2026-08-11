import { createContext, useContext, useEffect, useRef, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { locationKeys } from '../features/locations/queries';
import { staffProfilesApi } from '../features/staff-profiles/api';
import { hasStaffPermission } from '../features/staff-profiles/capabilities';
import { staffProfileKeys } from '../features/staff-profiles/queries';
import type { StaffProfile } from '../features/staff-profiles/types';
import { storageAreaKeys } from '../features/storage-areas/queries';
import { teamKeys } from '../features/team/queries';
import { masterDataKeys } from '../features/master-data/queries';

interface StaffProfileContextValue {
  profile: StaffProfile | null;
  loading: boolean;
  error: unknown;
  missing: boolean;
  hasPermission: (permission: string) => boolean;
}

const StaffProfileContext = createContext<StaffProfileContextValue | null>(null);

export function isMissingStaffProfileError(error: unknown): boolean {
  if (!axios.isAxiosError(error) || error.response?.status !== 404) return false;
  const data = error.response.data;
  return !data || typeof data !== 'object' || !('code' in data) || data.code === 'NOT_FOUND';
}

export function StaffProfileProvider({ principalId, tenantId, enabled, children }: { principalId: string; tenantId: string; enabled: boolean; children: ReactNode }) {
  const queryClient = useQueryClient();
  const previousPrincipal = useRef<string | null>(null);

  useEffect(() => {
    if (previousPrincipal.current !== null && previousPrincipal.current !== principalId) {
      const previousId = previousPrincipal.current;
      const belongsToPreviousPrincipal = (query: { queryKey: readonly unknown[] }) => query.queryKey[2] === previousId;
      queryClient.removeQueries({ queryKey: staffProfileKeys.all, predicate: belongsToPreviousPrincipal });
      queryClient.removeQueries({ queryKey: locationKeys.all, predicate: belongsToPreviousPrincipal });
      queryClient.removeQueries({ queryKey: storageAreaKeys.all, predicate: belongsToPreviousPrincipal });
      queryClient.removeQueries({ queryKey: teamKeys.all, predicate: (query) => query.queryKey[2] === previousId });
      queryClient.removeQueries({ queryKey: masterDataKeys.all, predicate: belongsToPreviousPrincipal });
    }
    previousPrincipal.current = principalId;
  }, [principalId, queryClient]);

  const profileQuery = useQuery({
    queryKey: staffProfileKeys.self(principalId, tenantId),
    queryFn: () => staffProfilesApi.getSelf(),
    enabled: enabled && !!principalId && !!tenantId,
    retry: (count, error) => !isMissingStaffProfileError(error) && count < 1,
  });
  const profile = enabled ? profileQuery.data?.staffProfile ?? null : null;
  const missing = profileQuery.isError && isMissingStaffProfileError(profileQuery.error);

  return <StaffProfileContext.Provider value={{
    profile,
    loading: enabled && profileQuery.isPending,
    error: enabled && !missing ? profileQuery.error : null,
    missing,
    hasPermission: (permission) => hasStaffPermission(profile, permission),
  }}>{children}</StaffProfileContext.Provider>;
}

export function useStaffProfile() {
  const context = useContext(StaffProfileContext);
  if (!context) throw new Error('useStaffProfile must be used within a StaffProfileProvider');
  return context;
}
