import { useEffect, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { Building2, CheckCircle2, ChevronRight, MapPin, RefreshCw, Settings2 } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import { tenantApi } from '../../api/client';
import { useAuth } from '../../contexts/AuthContext';
import { useActiveLocation } from '../../contexts/ActiveLocationContext';
import { useTenant } from '../../contexts/TenantContext';
import { getErrorMessage } from '../../utils/errors';
import { locationsApi } from '../../features/locations/api';
import { locationKeys } from '../../features/locations/queries';
import type { CreateLocationInput } from '../../features/locations/types';
import { getTimezoneOptions, validateLocation, type LocationValidationErrors } from '../../features/locations/validation';
import { restaurantSettingsApi } from '../../features/restaurant-settings/api';
import { restaurantSettingsKeys } from '../../features/restaurant-settings/queries';
import type { RestaurantSettingsFields, RestaurantSettingsValidationErrors } from '../../features/restaurant-settings/validation';
import { normalizeRestaurantSettings, validateRestaurantSettings } from '../../features/restaurant-settings/validation';
import { onboardingApi } from '../../features/onboarding/api';
import { onboardingKeys } from '../../features/onboarding/queries';

type Step = 'restaurant' | 'location' | 'finish';
const DEFAULT_SETTINGS: RestaurantSettingsFields = { currency: 'EUR', language: 'de-DE', defaultTimezone: 'Europe/Berlin' };

export default function OnboardingPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user, refreshUser } = useAuth();
  const { activeTenant } = useTenant();
  const { locations, loading: locationsLoading, error: locationsError } = useActiveLocation();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const [step, setStep] = useState<Step>('restaurant');
  const [restaurantName, setRestaurantName] = useState(activeTenant?.tenantName ?? '');
  const [restaurantNameError, setRestaurantNameError] = useState('');
  const [settings, setSettings] = useState<RestaurantSettingsFields>(DEFAULT_SETTINGS);
  const [settingsVersion, setSettingsVersion] = useState(0);
  const [settingsErrors, setSettingsErrors] = useState<RestaurantSettingsValidationErrors>({});
  const [location, setLocation] = useState<CreateLocationInput>({ code: '', name: '', timezone: DEFAULT_SETTINGS.defaultTimezone });
  const [locationErrors, setLocationErrors] = useState<LocationValidationErrors>({});
  const timezoneOptions = getTimezoneOptions();

  const settingsQuery = useQuery({
    queryKey: restaurantSettingsKeys.detail(tenantId),
    queryFn: restaurantSettingsApi.get,
    enabled: !!principalId && !!tenantId,
  });
  const onboardingQuery = useQuery({
    queryKey: onboardingKeys.detail(principalId, tenantId),
    queryFn: onboardingApi.get,
    enabled: !!principalId && !!tenantId,
  });
  useEffect(() => {
    const status = onboardingQuery.data?.onboarding;
    if (!status || status.completed) return;
    if (status.restaurantSettingsComplete && status.firstLocationComplete) setStep('finish');
    else if (status.restaurantSettingsComplete) setStep('location');
    else setStep('restaurant');
  }, [onboardingQuery.data]);
  useEffect(() => {
    if (!settingsQuery.data?.settings) return;
    const current = settingsQuery.data.settings;
    setSettings({ currency: current.currency, language: current.language, defaultTimezone: current.defaultTimezone });
    setSettingsVersion(current.version);
    setLocation((value) => ({ ...value, timezone: value.timezone || current.defaultTimezone }));
  }, [settingsQuery.data]);
  useEffect(() => setRestaurantName(activeTenant?.tenantName ?? ''), [activeTenant?.tenantName]);

  const saveRestaurant = useMutation({
    mutationFn: async () => {
      const normalized = normalizeRestaurantSettings(settings);
      if (restaurantName.trim() !== activeTenant?.tenantName) await tenantApi.updateSettings({ name: restaurantName.trim() });
      const response = await restaurantSettingsApi.update({ ...normalized, version: settingsVersion });
      return response.settings;
    },
    onSuccess: async (saved) => {
      queryClient.setQueryData(restaurantSettingsKeys.detail(tenantId), { settings: saved });
      await refreshUser();
      await queryClient.invalidateQueries({ queryKey: onboardingKeys.detail(principalId, tenantId), exact: true });
      setStep('location');
    },
  });
  const createLocation = useMutation({
    mutationFn: () => locationsApi.create({ code: location.code.trim(), name: location.name.trim(), timezone: location.timezone.trim() }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: locationKeys.list(principalId, tenantId), exact: true });
      await queryClient.invalidateQueries({ queryKey: onboardingKeys.detail(principalId, tenantId), exact: true });
      setStep('finish');
    },
  });
  const complete = useMutation({
    mutationFn: onboardingApi.complete,
    onSuccess: ({ onboarding }) => {
      queryClient.setQueryData(onboardingKeys.detail(principalId, tenantId), { onboarding });
      navigate('/dashboard', { replace: true });
    },
  });

  const submitRestaurant = (event: FormEvent) => {
    event.preventDefault();
    const errors = validateRestaurantSettings(settings);
    setRestaurantNameError(restaurantName.trim() ? '' : 'Restaurant name is required.');
    setSettingsErrors(errors);
    if (!restaurantName.trim() || Object.keys(errors).length) return;
    saveRestaurant.mutate();
  };
  const submitLocation = (event: FormEvent) => {
    event.preventDefault();
    if (locationsLoading || locationsError) return;
    if (locations.some((item) => item.isActive)) {
      setStep('finish');
      return;
    }
    const errors = validateLocation(location);
    setLocationErrors(errors);
    if (Object.keys(errors).length) return;
    createLocation.mutate();
  };

  const steps: Array<{ key: Step; label: string; icon: typeof Building2 }> = [
    { key: 'restaurant', label: 'Restaurant', icon: Building2 },
    { key: 'location', label: 'First location', icon: MapPin },
    { key: 'finish', label: 'Ready', icon: CheckCircle2 },
  ];
  const currentIndex = steps.findIndex((item) => item.key === step);
  const error = saveRestaurant.error || createLocation.error || complete.error;

  return <main className="min-h-screen bg-dark-950 px-4 py-10 sm:py-16">
    <div className="mx-auto w-full max-w-2xl">
      <div className="mb-8"><p className="mb-2 text-xs font-semibold uppercase tracking-[0.22em] text-primary-400">Restaurant setup</p><h1 className="text-3xl font-bold tracking-tight text-white">Build your operating workspace</h1><p className="mt-2 text-dark-400">Set the defaults once. Every step is saved independently, so you can safely resume after an interruption.</p></div>
      <ol className="mb-8 grid grid-cols-3 gap-2">{steps.map((item, index) => <li key={item.key} className={`rounded-xl border p-3 ${index === currentIndex ? 'border-primary-500/60 bg-primary-500/10' : index < currentIndex ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-dark-800 bg-dark-900/40'}`}><div className="flex items-center gap-2"><item.icon className={`h-4 w-4 ${index <= currentIndex ? 'text-primary-400' : 'text-dark-600'}`} /><span className="text-xs font-medium text-dark-300 sm:text-sm">{item.label}</span></div></li>)}</ol>
      {error && <Alert className="mb-5">{getErrorMessage(error)}</Alert>}

      {step === 'restaurant' && <form onSubmit={submitRestaurant} className="space-y-5 rounded-2xl border border-dark-800 bg-dark-900/60 p-6">
        <div><h2 className="text-xl font-semibold text-white">Restaurant basics</h2><p className="mt-1 text-sm text-dark-400">These defaults drive reports, imports, and new locations.</p></div>
        {settingsQuery.isPending ? <div className="flex justify-center py-12"><RefreshCw className="h-6 w-6 animate-spin text-primary-400" /></div> : <>
          <Input label="Restaurant company name" value={restaurantName} onChange={(event) => { setRestaurantName(event.target.value); setRestaurantNameError(''); }} error={restaurantNameError} maxLength={200} disabled={saveRestaurant.isPending} />
          <div className="grid gap-4 sm:grid-cols-2"><Input label="Currency" value={settings.currency} onChange={(event) => setSettings((value) => ({ ...value, currency: event.target.value }))} error={settingsErrors.currency} maxLength={3} disabled={saveRestaurant.isPending} /><Input label="Language" value={settings.language} onChange={(event) => setSettings((value) => ({ ...value, language: event.target.value }))} error={settingsErrors.language} placeholder="de-DE" disabled={saveRestaurant.isPending} /></div>
          <div><label htmlFor="onboarding-timezone" className="mb-1 block text-sm font-medium text-dark-300">Default timezone</label><Input id="onboarding-timezone" list="onboarding-timezones" value={settings.defaultTimezone} onChange={(event) => setSettings((value) => ({ ...value, defaultTimezone: event.target.value }))} error={settingsErrors.defaultTimezone} disabled={saveRestaurant.isPending} /><datalist id="onboarding-timezones">{timezoneOptions.map((timezone) => <option key={timezone} value={timezone} />)}</datalist></div>
          <Button type="submit" disabled={saveRestaurant.isPending || settingsQuery.isError} className="flex w-full items-center justify-center gap-2">{saveRestaurant.isPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <ChevronRight className="h-4 w-4" />}Save and continue</Button>
        </>}
      </form>}

      {step === 'location' && <form onSubmit={submitLocation} className="space-y-5 rounded-2xl border border-dark-800 bg-dark-900/60 p-6">
        <div><h2 className="text-xl font-semibold text-white">{locations.some((item) => item.isActive) ? 'Location already configured' : 'Create your first location'}</h2><p className="mt-1 text-sm text-dark-400">Inventory, storage, staff access, and forecasting are always scoped to a location.</p></div>
        {locationsLoading ? <div className="flex justify-center py-10"><RefreshCw className="h-6 w-6 animate-spin text-primary-400" /></div> : locationsError ? <Alert>Locations could not be loaded. Retry before creating another location.</Alert> : locations.some((item) => item.isActive) ? <Alert variant="success">{locations.find((item) => item.isActive)?.name} is ready. Continue without creating a duplicate.</Alert> : <><div className="grid gap-4 sm:grid-cols-2"><Input label="Location code" value={location.code} onChange={(event) => setLocation((value) => ({ ...value, code: event.target.value.toLowerCase() }))} error={locationErrors.code} placeholder="berlin-mitte" disabled={createLocation.isPending} /><Input label="Location name" value={location.name} onChange={(event) => setLocation((value) => ({ ...value, name: event.target.value }))} error={locationErrors.name} placeholder="Berlin Mitte" disabled={createLocation.isPending} /></div><Input label="Location timezone" list="onboarding-timezones" value={location.timezone} onChange={(event) => setLocation((value) => ({ ...value, timezone: event.target.value }))} error={locationErrors.timezone} disabled={createLocation.isPending} /></>}
        <div className="flex gap-2"><Button type="button" variant="secondary" onClick={() => setStep('restaurant')} disabled={createLocation.isPending}>Back</Button>{locationsError ? <Button type="button" onClick={() => queryClient.invalidateQueries({ queryKey: locationKeys.list(principalId, tenantId), exact: true })} className="flex-1">Retry locations</Button> : <Button type="submit" disabled={createLocation.isPending || locationsLoading} className="flex flex-1 items-center justify-center gap-2">{createLocation.isPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <ChevronRight className="h-4 w-4" />}Continue</Button>}</div>
      </form>}

      {step === 'finish' && <section className="rounded-2xl border border-dark-800 bg-dark-900/60 p-6 text-center"><div className="mx-auto mb-5 flex h-14 w-14 items-center justify-center rounded-2xl bg-emerald-500/15"><CheckCircle2 className="h-7 w-7 text-emerald-400" /></div><h2 className="text-xl font-semibold text-white">Your restaurant foundation is ready</h2><p className="mx-auto mt-2 max-w-md text-sm text-dark-400">Next, add storage areas, invite staff, and publish brand assets from workspace settings.</p><div className="mt-6 rounded-xl border border-dark-800 bg-dark-950/50 p-4 text-left text-sm text-dark-300"><div className="flex items-center gap-2"><Settings2 className="h-4 w-4 text-primary-400" />Restaurant defaults saved</div><div className="mt-2 flex items-center gap-2"><MapPin className="h-4 w-4 text-primary-400" />At least one active location</div></div><Button onClick={() => complete.mutate()} disabled={complete.isPending} className="mt-6 flex w-full items-center justify-center gap-2">{complete.isPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <CheckCircle2 className="h-4 w-4" />}Complete setup</Button></section>}
    </div>
  </main>;
}
