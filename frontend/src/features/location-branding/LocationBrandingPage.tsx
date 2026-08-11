import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { AlertCircle, CheckCircle2, LockKeyhole, MapPin, Paintbrush, RefreshCw, RotateCcw, Save } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import WorkspaceSettingsNav from '../../components/WorkspaceSettingsNav';
import { useAuth } from '../../contexts/AuthContext';
import { useActiveLocation } from '../../contexts/ActiveLocationContext';
import { useTenant } from '../../contexts/TenantContext';
import { getErrorMessage } from '../../utils/errors';
import { BRANDING_FONTS, getBrandingFontStack } from '../tenant-branding/validation';
import { locationBrandingApi } from './api';
import { locationBrandingKeys } from './queries';
import type { LocationBrandingResponse, UpdateLocationBrandingInput } from './types';
import { normalizeLocationBranding, validateLocationBranding, type LocationBrandingFields, type LocationBrandingValidationErrors } from './validation';

const EMPTY_FORM: LocationBrandingFields = { displayName: '', primaryColor: '', accentColor: '', font: '' };

function isVersionConflict(error: unknown) {
  return axios.isAxiosError(error) && error.response?.data?.code === 'VERSION_CONFLICT';
}

export default function LocationBrandingPage() {
  const { user } = useAuth();
  const { activeTenant, role, isRootTenant } = useTenant();
  const { activeLocation, locations, loading: locationsLoading } = useActiveLocation();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const locationId = activeLocation?.id ?? '';
  const scopeKey = `${principalId}:${tenantId}:${locationId}`;
  const scope = useRef(scopeKey);
  const canWrite = !isRootTenant && (role === 'owner' || role === 'admin');
  const queryClient = useQueryClient();
  const [form, setForm] = useState<LocationBrandingFields>(EMPTY_FORM);
  const [version, setVersion] = useState(0);
  const [errors, setErrors] = useState<LocationBrandingValidationErrors>({});
  const [success, setSuccess] = useState('');

  const detailKey = locationBrandingKeys.detail(principalId, tenantId, locationId);
  const brandingQuery = useQuery({
    queryKey: detailKey,
    queryFn: () => locationBrandingApi.get(locationId),
    enabled: !!principalId && !!tenantId && !!locationId && !isRootTenant,
  });
  const updateBranding = useMutation({
    mutationFn: ({ locationId, input }: { principalId: string; tenantId: string; locationId: string; input: UpdateLocationBrandingInput }) => locationBrandingApi.update(locationId, input),
    onSuccess: async ({ branding }, variables) => {
      const key = locationBrandingKeys.detail(variables.principalId, variables.tenantId, variables.locationId);
      queryClient.setQueryData<LocationBrandingResponse>(key, (current) => current ? { ...current, branding } : current);
      await queryClient.invalidateQueries({ queryKey: key, exact: true });
      if (scope.current !== `${variables.principalId}:${variables.tenantId}:${variables.locationId}`) return;
      const latest = queryClient.getQueryData<LocationBrandingResponse>(key)?.branding ?? branding;
      setForm({ displayName: latest.displayName, primaryColor: latest.primaryColor, accentColor: latest.accentColor, font: latest.font });
      setVersion(latest.version);
      setErrors({});
      setSuccess('Location branding published.');
    },
    onError: async (error, variables) => {
      if (isVersionConflict(error)) await queryClient.invalidateQueries({ queryKey: locationBrandingKeys.detail(variables.principalId, variables.tenantId, variables.locationId), exact: true });
    },
  });
  const resetBranding = useMutation({
    mutationFn: ({ locationId, version }: { principalId: string; tenantId: string; locationId: string; version: number }) => locationBrandingApi.reset(locationId, version),
    onSuccess: async (_, variables) => {
      const key = locationBrandingKeys.detail(variables.principalId, variables.tenantId, variables.locationId);
      await queryClient.invalidateQueries({ queryKey: key, exact: true });
      if (scope.current !== `${variables.principalId}:${variables.tenantId}:${variables.locationId}`) return;
      setSuccess('Location override removed. Tenant branding now applies.');
    },
    onError: async (error, variables) => {
      if (isVersionConflict(error)) await queryClient.invalidateQueries({ queryKey: locationBrandingKeys.detail(variables.principalId, variables.tenantId, variables.locationId), exact: true });
    },
  });

  const resetUpdate = updateBranding.reset;
  const resetDelete = resetBranding.reset;
  useLayoutEffect(() => {
    scope.current = scopeKey;
    setForm(EMPTY_FORM);
    setVersion(0);
    setErrors({});
    setSuccess('');
    resetUpdate();
    resetDelete();
  }, [scopeKey, resetDelete, resetUpdate]);

  useEffect(() => {
    const branding = brandingQuery.data?.branding;
    if (!branding) return;
    setForm({ displayName: branding.displayName, primaryColor: branding.primaryColor, accentColor: branding.accentColor, font: branding.font });
    setVersion(branding.version);
  }, [brandingQuery.data]);

  const setField = (field: keyof LocationBrandingFields, value: string) => {
    setForm((current) => ({ ...current, [field]: value }));
    setErrors((current) => ({ ...current, [field]: undefined }));
    setSuccess('');
    updateBranding.reset();
  };
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    const validationErrors = validateLocationBranding(form);
    setErrors(validationErrors);
    if (Object.keys(validationErrors).length || !canWrite || !locationId) return;
    updateBranding.mutate({ principalId, tenantId, locationId, input: { ...normalizeLocationBranding(form), version } });
  };
  const handleReset = () => {
    if (!canWrite || !locationId || version < 1 || !window.confirm('Remove this location override and inherit tenant branding?')) return;
    resetBranding.mutate({ principalId, tenantId, locationId, version });
  };

  const mutationInScope = updateBranding.variables?.locationId === locationId;
  const resetInScope = resetBranding.variables?.locationId === locationId;
  const pending = mutationInScope && updateBranding.isPending || resetInScope && resetBranding.isPending;
  const mutation = updateBranding.isError ? updateBranding : resetBranding;
  const mutationError = mutation.isError && (mutation.variables?.locationId === locationId)
    ? isVersionConflict(mutation.error) ? 'Location branding changed elsewhere. The latest version has been loaded; review and try again.' : getErrorMessage(mutation.error)
    : '';
  const resolved = brandingQuery.data?.resolved;
  const previewPrimary = /^#[0-9a-fA-F]{6}$/.test(form.primaryColor) ? form.primaryColor : resolved?.primaryColor || '#0ea5e9';
  const previewAccent = /^#[0-9a-fA-F]{6}$/.test(form.accentColor) ? form.accentColor : resolved?.accentColor || '#a855f7';
  const previewFont = getBrandingFontStack(form.font || resolved?.font || '') || 'Inter, system-ui, sans-serif';
  const previewName = form.displayName.trim() || resolved?.displayName || activeLocation?.name || 'Location';

  return <div className="space-y-8">
    <header><div className="mb-2 flex items-center gap-2 text-sm font-medium text-primary-400"><Paintbrush className="h-4 w-4" />Workspace settings</div><h1 className="text-3xl font-bold tracking-tight text-white">Location branding</h1><p className="mt-2 max-w-2xl text-dark-400">Override safe visual tokens for the active location. Empty fields inherit restaurant branding and then platform defaults.</p></header>
    <WorkspaceSettingsNav />
    {!canWrite && !isRootTenant && <Alert variant="info" className="flex items-start gap-2 p-4"><LockKeyhole className="mt-0.5 h-4 w-4 shrink-0" /><span><strong className="font-semibold">Read-only access.</strong> Owners and admins with an eligible plan can publish location overrides.</span></Alert>}
    {success && <Alert variant="success" className="flex items-start gap-2"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />{success}</Alert>}
    {mutationError && <Alert className="flex items-start gap-2"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />{mutationError}</Alert>}
    <section className="overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/60">
      <div className="border-b border-dark-800 px-5 py-4 sm:px-6"><h2 className="font-semibold text-white">Active location override</h2><p className="mt-0.5 text-xs text-dark-500">Published version {version}. Active location: {activeLocation?.name ?? 'none selected'}.</p></div>
      {isRootTenant ? <EmptyState icon={LockKeyhole} title="Location branding is tenant-only" body="Select a restaurant workspace to manage location overrides." />
        : locationsLoading ? <div className="space-y-5 p-6" aria-label="Loading locations">{[1, 2, 3].map((item) => <div key={item} className="h-16 animate-pulse rounded-xl bg-dark-800/70" />)}</div>
        : locations.length === 0 ? <EmptyState icon={MapPin} title="Create a location first" body="Location branding becomes available after the workspace has an active location." />
        : !activeLocation ? <EmptyState icon={MapPin} title="Select an active location" body="Choose a location from the header selector to view and edit its branding." />
        : brandingQuery.isPending ? <div className="space-y-5 p-6" aria-label="Loading location branding">{[1, 2, 3].map((item) => <div key={item} className="h-16 animate-pulse rounded-xl bg-dark-800/70" />)}</div>
        : brandingQuery.isError ? <div className="flex flex-col items-center px-6 py-16 text-center"><AlertCircle className="mb-4 h-9 w-9 text-red-400" /><h3 className="font-semibold text-white">Could not load location branding</h3><p className="mt-2 max-w-md text-sm text-dark-400">{getErrorMessage(brandingQuery.error)}</p><Button variant="secondary" className="mt-5 inline-flex items-center gap-2" onClick={() => brandingQuery.refetch()}><RefreshCw className="h-4 w-4" />Retry</Button></div>
        : <form className="grid gap-8 p-5 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.8fr)] sm:p-6" onSubmit={handleSubmit} noValidate>
          <div className="space-y-5"><Input label="Location display name" value={form.displayName} onChange={(event) => setField('displayName', event.target.value)} error={errors.displayName} placeholder={resolved?.displayName || activeLocation.name} maxLength={200} disabled={!canWrite || pending} /><div className="grid gap-5 sm:grid-cols-2"><Input label="Primary color" value={form.primaryColor} onChange={(event) => setField('primaryColor', event.target.value)} error={errors.primaryColor} placeholder={resolved?.primaryColor || '#0ea5e9'} maxLength={7} disabled={!canWrite || pending} /><Input label="Accent color" value={form.accentColor} onChange={(event) => setField('accentColor', event.target.value)} error={errors.accentColor} placeholder={resolved?.accentColor || '#a855f7'} maxLength={7} disabled={!canWrite || pending} /></div><div><label htmlFor="location-branding-font" className="mb-1 block text-sm font-medium text-dark-300">Font style</label><select id="location-branding-font" value={form.font} onChange={(event) => setField('font', event.target.value)} disabled={!canWrite || pending} className="w-full rounded-lg border border-dark-700 bg-dark-800 px-3 py-2 text-sm text-white focus:border-primary-500 focus:outline-none disabled:opacity-50">{BRANDING_FONTS.map((font) => <option key={font.value || 'default'} value={font.value}>{font.value ? font.label : `Inherit (${resolved?.font || 'platform'})`}</option>)}</select>{errors.font && <p className="mt-1 text-xs text-red-400">{errors.font}</p>}</div>{canWrite && <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end"><Button type="button" variant="secondary" disabled={pending || version < 1} onClick={handleReset} className="inline-flex items-center justify-center gap-2"><RotateCcw className="h-4 w-4" />Remove override</Button><Button type="submit" disabled={pending} className="inline-flex items-center justify-center gap-2">{pending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}{pending ? 'Publishing...' : 'Publish override'}</Button></div>}</div>
          <div><p className="mb-2 text-xs font-semibold uppercase tracking-[0.16em] text-dark-500">Resolved preview</p><div className="overflow-hidden rounded-2xl border border-dark-700 bg-dark-950 shadow-2xl" style={{ fontFamily: previewFont }}><div className="h-2" style={{ background: `linear-gradient(90deg, ${previewPrimary}, ${previewAccent})` }} /><div className="p-6"><p className="text-lg font-semibold text-white">{previewName}</p><p className="mt-1 text-xs text-dark-500">{activeLocation.code} · {activeLocation.timezone}</p><p className="mt-5 text-sm text-dark-400">Location values win; empty fields inherit automatically.</p><div className="mt-5 inline-flex rounded-lg px-4 py-2 text-sm font-semibold text-white" style={{ backgroundColor: previewPrimary }}>Primary action</div><span className="ml-3 text-sm font-medium" style={{ color: previewAccent }}>Accent</span></div></div><dl className="mt-4 grid grid-cols-2 gap-2 text-xs">{resolved && Object.entries(resolved.sources).map(([field, source]) => <div key={field} className="rounded-lg bg-dark-950/60 p-2"><dt className="capitalize text-dark-500">{field}</dt><dd className="mt-0.5 text-dark-300">{source.replace('_', ' ')}</dd></div>)}</dl></div>
        </form>}
    </section>
  </div>;
}

function EmptyState({ icon: Icon, title, body }: { icon: typeof MapPin; title: string; body: string }) {
  return <div className="flex flex-col items-center px-6 py-16 text-center"><Icon className="mb-4 h-9 w-9 text-dark-500" /><h3 className="font-semibold text-white">{title}</h3><p className="mt-2 max-w-md text-sm text-dark-400">{body}</p></div>;
}
