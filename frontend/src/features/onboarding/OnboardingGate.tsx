import { useQuery } from '@tanstack/react-query';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import LoadingSpinner from '../../components/LoadingSpinner';
import { useAuth } from '../../contexts/AuthContext';
import { useTenant } from '../../contexts/TenantContext';
import { onboardingApi } from './api';
import { onboardingKeys } from './queries';

export default function OnboardingGate() {
  const location = useLocation();
  const { user, memberships } = useAuth();
  const { activeTenant, role, isRootTenant } = useTenant();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const ownerWorkspace = !!activeTenant && !isRootTenant && role === 'owner';
  const billingRecoveryRoute = location.pathname === '/plan' || location.pathname === '/buy-credits' || location.pathname.startsWith('/billing/');
  const onboardingQuery = useQuery({
    queryKey: onboardingKeys.detail(principalId, tenantId),
    queryFn: onboardingApi.get,
    enabled: !!principalId && !!tenantId && ownerWorkspace,
  });

  if (user && memberships.length > 0 && !activeTenant) {
    return <div className="flex min-h-screen items-center justify-center bg-dark-950"><LoadingSpinner size="lg" /></div>;
  }
  if (!ownerWorkspace) {
    return location.pathname === '/onboarding' ? <Navigate to="/dashboard" replace /> : <Outlet />;
  }
  if (onboardingQuery.isPending) {
    return <div className="flex min-h-screen items-center justify-center bg-dark-950"><LoadingSpinner size="lg" /></div>;
  }
  if (onboardingQuery.isError) {
    return <div className="flex min-h-screen items-center justify-center bg-dark-950 px-4"><div className="max-w-md rounded-2xl border border-dark-800 bg-dark-900 p-6 text-center"><h1 className="font-semibold text-white">Setup status unavailable</h1><p className="mt-2 text-sm text-dark-400">We could not safely determine whether this restaurant is ready.</p><button type="button" onClick={() => onboardingQuery.refetch()} className="mt-5 rounded-lg bg-primary-500 px-4 py-2 text-sm font-semibold text-white">Retry</button></div></div>;
  }
  if (!onboardingQuery.data?.onboarding.completed && location.pathname !== '/onboarding' && !billingRecoveryRoute) {
    return <Navigate to="/onboarding" replace state={{ from: location.pathname }} />;
  }
  if (onboardingQuery.data?.onboarding.completed && location.pathname === '/onboarding') {
    return <Navigate to="/dashboard" replace />;
  }
  return <Outlet />;
}
