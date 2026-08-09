import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from './AuthContext';
import { useTenant } from './TenantContext';
import { tenantBrandingApi } from '../features/tenant-branding/api';
import { tenantBrandingKeys } from '../features/tenant-branding/queries';
import type { TenantBranding, TenantBrandingAsset, TenantBrandingAssetKind } from '../features/tenant-branding/types';

interface TenantBrandingContextValue {
  branding: TenantBranding | null;
  loading: boolean;
  error: unknown;
  assets: TenantBrandingAsset[];
  assetsLoading: boolean;
  assetsError: unknown;
  primaryLogoUrl: string | null;
  compactLogoUrl: string | null;
}

const TenantBrandingContext = createContext<TenantBrandingContextValue | null>(null);

function useScopedObjectUrl(blob: Blob | undefined, scopeKey: string): string | null {
  const [state, setState] = useState<{ scopeKey: string; url: string } | null>(null);
  useEffect(() => {
    if (!blob) {
      setState(null);
      return;
    }
    const url = URL.createObjectURL(blob);
    setState({ scopeKey, url });
    return () => URL.revokeObjectURL(url);
  }, [blob, scopeKey]);
  return state?.scopeKey === scopeKey ? state.url : null;
}

export function TenantBrandingProvider({ children }: { children: ReactNode }) {
  const { user, isAuthenticated } = useAuth();
  const { activeTenant, isRootTenant } = useTenant();
  const queryClient = useQueryClient();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const enabled = isAuthenticated && !!principalId && !!tenantId && !isRootTenant;
  const previousScope = useRef<{ principalId: string; tenantId: string } | null>(null);

  useEffect(() => {
    const previous = previousScope.current;
    if (previous && (previous.principalId !== principalId || previous.tenantId !== tenantId)) {
      queryClient.removeQueries({
        queryKey: tenantBrandingKeys.all,
        predicate: (query) => query.queryKey[2] === previous.principalId && query.queryKey[3] === previous.tenantId,
      });
    }
    previousScope.current = principalId && tenantId ? { principalId, tenantId } : null;
  }, [principalId, queryClient, tenantId]);

  const brandingQuery = useQuery({
    queryKey: tenantBrandingKeys.detail(principalId, tenantId),
    queryFn: () => tenantBrandingApi.get(),
    enabled,
  });
  const assetsQuery = useQuery({
    queryKey: tenantBrandingKeys.assets(principalId, tenantId),
    queryFn: () => tenantBrandingApi.listAssets(),
    enabled,
  });
  const assets = enabled ? assetsQuery.data?.assets ?? [] : [];
  const getAsset = (kind: TenantBrandingAssetKind) => assets.find((asset) => asset.kind === kind);
  const primary = getAsset('primary');
  const compact = getAsset('compact');
  const primaryQuery = useQuery({
    queryKey: tenantBrandingKeys.asset(principalId, tenantId, 'primary', primary?.version ?? 0),
    queryFn: () => tenantBrandingApi.getAsset('primary'),
    enabled: enabled && !!primary,
  });
  const compactQuery = useQuery({
    queryKey: tenantBrandingKeys.asset(principalId, tenantId, 'compact', compact?.version ?? 0),
    queryFn: () => tenantBrandingApi.getAsset('compact'),
    enabled: enabled && !!compact,
  });
  const scopeKey = enabled ? `${principalId}:${tenantId}` : '';
  const primaryLogoUrl = useScopedObjectUrl(enabled ? primaryQuery.data : undefined, `${scopeKey}:primary:${primary?.version ?? 0}`);
  const compactLogoUrl = useScopedObjectUrl(enabled ? compactQuery.data : undefined, `${scopeKey}:compact:${compact?.version ?? 0}`);

  return (
    <TenantBrandingContext.Provider value={{
      branding: enabled ? brandingQuery.data?.branding ?? null : null,
      loading: enabled && brandingQuery.isPending,
      error: enabled ? brandingQuery.error : null,
      assets,
      assetsLoading: enabled && assetsQuery.isPending,
      assetsError: enabled ? assetsQuery.error : null,
      primaryLogoUrl,
      compactLogoUrl,
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
