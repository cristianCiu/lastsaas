import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { AlertCircle, CheckCircle2, Globe2, LockKeyhole, RefreshCw, Save, WalletCards } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import WorkspaceSettingsNav from '../../components/WorkspaceSettingsNav';
import { useTenant } from '../../contexts/TenantContext';
import { getErrorMessage } from '../../utils/errors';
import { getTimezoneOptions } from '../locations/validation';
import { restaurantSettingsApi } from './api';
import { restaurantSettingsKeys } from './queries';
import type { RestaurantSettings, UpdateRestaurantSettingsInput } from './types';
import { normalizeRestaurantSettings, validateRestaurantSettings, type RestaurantSettingsFields, type RestaurantSettingsValidationErrors } from './validation';

const DEFAULT_FORM: RestaurantSettingsFields = { currency: 'EUR', language: 'de', defaultTimezone: 'Europe/Berlin' };
const TIMEZONES = getTimezoneOptions();

function isVersionConflict(error: unknown) {
  return axios.isAxiosError(error) && error.response?.data?.code === 'VERSION_CONFLICT';
}

export default function RestaurantSettingsPage() {
  const { activeTenant, role } = useTenant();
  const tenantId = activeTenant?.tenantId ?? '';
  const canWrite = role === 'owner' || role === 'admin';
  const currentTenantId = useRef(tenantId);
  const queryClient = useQueryClient();
  const [form, setForm] = useState<RestaurantSettingsFields>(DEFAULT_FORM);
  const [version, setVersion] = useState(0);
  const [errors, setErrors] = useState<RestaurantSettingsValidationErrors>({});
  const [success, setSuccess] = useState('');

  const settingsQuery = useQuery({
    queryKey: restaurantSettingsKeys.detail(tenantId),
    queryFn: () => restaurantSettingsApi.get(),
    enabled: !!tenantId,
  });

  const updateSettings = useMutation({
    mutationFn: ({ input }: { tenantId: string; input: UpdateRestaurantSettingsInput }) => restaurantSettingsApi.update(input),
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: restaurantSettingsKeys.detail(variables.tenantId), exact: true });
      if (currentTenantId.current === variables.tenantId) setSuccess('');
    },
    onSuccess: async ({ settings }, variables) => {
      queryClient.setQueryData<{ settings: RestaurantSettings }>(restaurantSettingsKeys.detail(variables.tenantId), { settings });
      await queryClient.invalidateQueries({ queryKey: restaurantSettingsKeys.detail(variables.tenantId), exact: true });
      if (currentTenantId.current !== variables.tenantId) return;
      const latest = queryClient.getQueryData<{ settings: RestaurantSettings }>(restaurantSettingsKeys.detail(variables.tenantId))?.settings ?? settings;
      setForm({ currency: latest.currency, language: latest.language, defaultTimezone: latest.defaultTimezone });
      setVersion(latest.version);
      setErrors({});
      setSuccess('Restaurant settings saved.');
    },
    onError: async (error, variables) => {
      if (!isVersionConflict(error)) return;
      await queryClient.invalidateQueries({ queryKey: restaurantSettingsKeys.detail(variables.tenantId), exact: true });
    },
  });

  const resetUpdateSettings = updateSettings.reset;

  useLayoutEffect(() => {
    currentTenantId.current = tenantId;
    setForm(DEFAULT_FORM);
    setVersion(0);
    setErrors({});
    setSuccess('');
    resetUpdateSettings();
  }, [tenantId, resetUpdateSettings]);

  useEffect(() => {
    if (!settingsQuery.data) return;
    const settings = settingsQuery.data.settings;
    setForm({ currency: settings.currency, language: settings.language, defaultTimezone: settings.defaultTimezone });
    setVersion(settings.version);
  }, [settingsQuery.data]);

  const setField = (field: keyof RestaurantSettingsFields, value: string) => {
    setForm((current) => ({ ...current, [field]: value }));
    setErrors((current) => ({ ...current, [field]: undefined }));
    setSuccess('');
    updateSettings.reset();
  };

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!canWrite || !tenantId) return;
    const validationErrors = validateRestaurantSettings(form);
    setErrors(validationErrors);
    if (Object.keys(validationErrors).length) return;
    const normalized = normalizeRestaurantSettings(form);
    updateSettings.mutate({ tenantId, input: { ...normalized, version } });
  };

  const mutationInScope = updateSettings.variables?.tenantId === tenantId;
  const mutationPending = mutationInScope && updateSettings.isPending;
  const mutationError = mutationInScope && updateSettings.isError
    ? isVersionConflict(updateSettings.error)
      ? 'These settings changed elsewhere. The latest version has been loaded; review it and save again.'
      : getErrorMessage(updateSettings.error)
    : '';

  return (
    <div className="space-y-8">
      <header>
        <div className="mb-2 flex items-center gap-2 text-sm font-medium text-primary-400"><Globe2 className="h-4 w-4" />Workspace settings</div>
        <h1 className="text-3xl font-bold tracking-tight text-white">Restaurant settings</h1>
        <p className="mt-2 max-w-2xl text-dark-400">Set operational defaults for every location in this workspace. Location-specific data can override these defaults where supported.</p>
      </header>

      <WorkspaceSettingsNav />

      <Alert variant="info" className="flex items-start gap-3 p-4">
        <WalletCards className="mt-0.5 h-4 w-4 shrink-0" />
        <span>Currency, language, and timezone apply to <strong className="font-semibold">{activeTenant?.tenantName ?? 'the active workspace'}</strong>. The company name is managed separately from these restaurant defaults.</span>
      </Alert>
      {!canWrite && <Alert variant="info" className="flex items-start gap-2 p-4"><LockKeyhole className="mt-0.5 h-4 w-4 shrink-0" /><span><strong className="font-semibold">Read-only access.</strong> Owners and admins can save restaurant settings.</span></Alert>}
      {success && <Alert variant="success" className="flex items-start gap-2"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />{success}</Alert>}
      {mutationError && <Alert className="flex items-start gap-2"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />{mutationError}</Alert>}

      <section className="overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/60">
        <div className="border-b border-dark-800 px-5 py-4 sm:px-6">
          <h2 className="font-semibold text-white">Operational defaults</h2>
          <p className="mt-0.5 text-xs text-dark-500">Saved with version {version}</p>
        </div>
        {!tenantId || settingsQuery.isPending ? (
          <div className="space-y-5 p-5 sm:p-6" aria-label="Loading restaurant settings">
            {[1, 2, 3].map((item) => <div key={item} className="h-16 animate-pulse rounded-xl bg-dark-800/70" />)}
          </div>
        ) : settingsQuery.isError ? (
          <div className="flex flex-col items-center px-6 py-16 text-center">
            <AlertCircle className="mb-4 h-9 w-9 text-red-400" />
            <h3 className="font-semibold text-white">Could not load restaurant settings</h3>
            <p className="mt-2 max-w-md text-sm text-dark-400">{getErrorMessage(settingsQuery.error)}</p>
            <Button variant="secondary" className="mt-5 inline-flex items-center gap-2" onClick={() => settingsQuery.refetch()}><RefreshCw className="h-4 w-4" />Retry</Button>
          </div>
        ) : (
          <form className="p-5 sm:p-6" onSubmit={handleSubmit} noValidate>
            <div className="grid gap-5 md:grid-cols-3">
               <Input label="Currency" value={form.currency} onChange={(event) => setField('currency', event.target.value)} error={errors.currency} placeholder="EUR" maxLength={3} disabled={!canWrite || mutationPending} />
               <Input label="Language" value={form.language} onChange={(event) => setField('language', event.target.value)} error={errors.language} placeholder="de" disabled={!canWrite || mutationPending} />
               <Input label="Default IANA timezone" value={form.defaultTimezone} onChange={(event) => setField('defaultTimezone', event.target.value)} error={errors.defaultTimezone} list="restaurant-timezones" placeholder="Europe/Berlin" disabled={!canWrite || mutationPending} />
              <datalist id="restaurant-timezones">{TIMEZONES.map((timezone) => <option key={timezone} value={timezone} />)}</datalist>
            </div>
            {canWrite && <div className="mt-6 flex justify-end"><Button type="submit" disabled={mutationPending} className="inline-flex w-full items-center justify-center gap-2 sm:w-auto">{mutationPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}{mutationPending ? 'Saving...' : 'Save settings'}</Button></div>}
          </form>
        )}
      </section>
    </div>
  );
}
