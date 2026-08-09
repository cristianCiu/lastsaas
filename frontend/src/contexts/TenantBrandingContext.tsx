import { createContext, useContext, useEffect, useRef, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from './AuthContext';
import { useTenant } from './TenantContext';
import { tenantBrandingApi } from '../features/tenant-branding/api';
import { tenantBrandingKeys } from '../features/tenant-branding/queries';
import type { TenantBranding } from '../features/tenant-branding/types';

interface TenantBrandingContextValue {
  branding: TenantBranding | null;
  loading: boolean;
  error: unknown;
}

const TenantBrandingContext = createContext<TenantBrandingContextValue | null>(null);

export function TenantBrandingProvider({ children }: { children: ReactNode }) {
  const { user, isAuthenticated } = useAuth();
  const { activeTenant, isRootTenant } = useTenant();
  const queryClient = useQueryClient();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const enabled = isAuthenticated && !!principalId && !!tenantId && !isRootTenant;
  const previousPrincipal = useRef<string | null>(null);

  useEffect(() => {
    if (previousPrincipal.current !== null && previousPrincipal.current !== principalId) {
      const previousId = previousPrincipal.current;
      queryClient.removeQueries({
        queryKey: tenantBrandingKeys.all,
        predicate: (query) => query.queryKey[2] === previousId,
      });
    }
    previousPrincipal.current = principalId;
  }, [principalId, queryClient]);

  const brandingQuery = useQuery({
    queryKey: tenantBrandingKeys.detail(principalId, tenantId),
    queryFn: () => tenantBrandingApi.get(),
    enabled,
  });

  return (
    <TenantBrandingContext.Provider value={{
      branding: enabled ? brandingQuery.data?.branding ?? null : null,
      loading: enabled && brandingQuery.isPending,
      error: enabled ? brandingQuery.error : null,
    }}>
      {children}
    </TenantBrandingContext.Provider>
  );
}

export function useTenantBranding() {
  const context = useContext(TenantBrandingContext);
  if (!context) throw new Error('useTenantBranding must be used within a TenantBrandingProvider');
  return context;
}
