import {useQuery} from '@tanstack/react-query';
import {useAuth} from '../../contexts/AuthContext';
import {useTenant} from '../../contexts/TenantContext';
import {useActiveLocation} from '../../contexts/ActiveLocationContext';
import {useStaffProfile} from '../../contexts/StaffProfileContext';
import ForecastPage from './ForecastPage';
import ForecastDashboard from './ForecastDashboard';
import RecommendationDraftActions from './RecommendationDraftActions';
import {forecastApi} from './api';
import {forecastKeys} from './queries';

export default function ForecastWorkspace() {
  const { user } = useAuth(); const { activeTenant, isRootTenant } = useTenant(); const { activeLocation } = useActiveLocation(); const { profile, hasPermission } = useStaffProfile();
  const p = user?.id ?? ''; const t = activeTenant?.tenantId ?? ''; const l = activeLocation?.id ?? ''; const read = !isRootTenant && hasPermission('forecast.read');
  const policies = useQuery({ queryKey: forecastKeys.policies(p, t, l), queryFn: () => forecastApi.policies(l), enabled: !!p && !!t && !!l && read });
  const runs = useQuery({ queryKey: forecastKeys.runs(p, t, l), queryFn: () => forecastApi.runs(l), enabled: !!p && !!t && !!l && read });
  const runId = runs.data?.forecastRuns.find(r => r.status === 'succeeded')?.id ?? '';
  const recommendations = useQuery({ queryKey: [...forecastKeys.runs(p, t, l), 'recommendations', runId], queryFn: () => forecastApi.recommendations(l, runId), enabled: !!runId && read });
  const canRun = read && hasPermission('forecast.manage') && hasPermission('forecast.run') && hasPermission('purchasing.manage') && (profile?.businessRole === 'company_owner' || profile?.businessRole === 'operations_manager');
  return <><ForecastPage /><div className="mx-auto max-w-7xl px-4 pb-8 sm:px-6 lg:px-8"><ForecastDashboard locationId={l} principalId={p} tenantId={t} policies={policies.data?.forecastPolicies ?? []} canRun={canRun} /><RecommendationDraftActions rows={recommendations.data?.recommendations ?? []} locationId={l} principalId={p} tenantId={t} enabled={canRun} /></div></>;
}
