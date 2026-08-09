import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { AlertCircle, CheckCircle2, LockKeyhole, Paintbrush, RefreshCw, RotateCcw, Save } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import WorkspaceSettingsNav from '../../components/WorkspaceSettingsNav';
import { useAuth } from '../../contexts/AuthContext';
import { useTenant } from '../../contexts/TenantContext';
import { getErrorMessage } from '../../utils/errors';
import { tenantBrandingApi } from './api';
import { tenantBrandingKeys } from './queries';
import type { TenantBranding, UpdateTenantBrandingInput } from './types';
import { BRANDING_FONTS, getBrandingFontStack, normalizeTenantBranding, validateTenantBranding, type TenantBrandingFields, type TenantBrandingValidationErrors } from './validation';

const DEFAULT_FORM: TenantBrandingFields = { primaryColor: '', accentColor: '', font: '' };

function isVersionConflict(error: unknown) {
  return axios.isAxiosError(error) && error.response?.data?.code === 'VERSION_CONFLICT';
}

export default function TenantBrandingPage() {
  const { user } = useAuth();
  const { activeTenant, role, isRootTenant } = useTenant();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const canWrite = !isRootTenant && (role === 'owner' || role === 'admin');
  const scopeKey = `${principalId}:${tenantId}`;
  const scope = useRef(scopeKey);
  const queryClient = useQueryClient();
  const [form, setForm] = useState<TenantBrandingFields>(DEFAULT_FORM);
  const [version, setVersion] = useState(0);
  const [errors, setErrors] = useState<TenantBrandingValidationErrors>({});
  const [success, setSuccess] = useState('');

  const brandingQuery = useQuery({
    queryKey: tenantBrandingKeys.detail(principalId, tenantId),
    queryFn: () => tenantBrandingApi.get(),
    enabled: !!principalId && !!tenantId && !isRootTenant,
  });
  const updateBranding = useMutation({
    mutationFn: ({ input }: { principalId: string; tenantId: string; input: UpdateTenantBrandingInput }) => tenantBrandingApi.update(input),
    onSuccess: async ({ branding }, variables) => {
      const key = tenantBrandingKeys.detail(variables.principalId, variables.tenantId);
      queryClient.setQueryData<{ branding: TenantBranding }>(key, { branding });
      await queryClient.invalidateQueries({ queryKey: key, exact: true });
      if (scope.current !== `${variables.principalId}:${variables.tenantId}`) return;
      const latest = queryClient.getQueryData<{ branding: TenantBranding }>(key)?.branding ?? branding;
      setForm({ primaryColor: latest.primaryColor, accentColor: latest.accentColor, font: latest.font });
      setVersion(latest.version);
      setErrors({});
      setSuccess('Branding published.');
    },
    onError: async (error, variables) => {
      if (isVersionConflict(error)) {
        await queryClient.invalidateQueries({ queryKey: tenantBrandingKeys.detail(variables.principalId, variables.tenantId), exact: true });
      }
    },
  });

  const resetMutation = updateBranding.reset;
  useLayoutEffect(() => {
    scope.current = scopeKey;
    setForm(DEFAULT_FORM);
    setVersion(0);
    setErrors({});
    setSuccess('');
    resetMutation();
  }, [scopeKey, resetMutation]);

  useEffect(() => {
    const branding = brandingQuery.data?.branding;
    if (!branding) return;
    setForm({ primaryColor: branding.primaryColor, accentColor: branding.accentColor, font: branding.font });
    setVersion(branding.version);
  }, [brandingQuery.data]);

  const setField = (field: keyof TenantBrandingFields, value: string) => {
    setForm((current) => ({ ...current, [field]: value }));
    setErrors((current) => ({ ...current, [field]: undefined }));
    setSuccess('');
    updateBranding.reset();
  };
  const publish = (fields: TenantBrandingFields) => {
    const validationErrors = validateTenantBranding(fields);
    setErrors(validationErrors);
    if (Object.keys(validationErrors).length || !canWrite || !tenantId) return;
    updateBranding.mutate({ principalId, tenantId, input: { ...normalizeTenantBranding(fields), version } });
  };
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    publish(form);
  };
  const resetToPlatform = () => {
    const defaults = { ...DEFAULT_FORM };
    setForm(defaults);
    publish(defaults);
  };

  const mutationInScope = updateBranding.variables?.principalId === principalId && updateBranding.variables?.tenantId === tenantId;
  const pending = mutationInScope && updateBranding.isPending;
  const mutationError = mutationInScope && updateBranding.isError
    ? isVersionConflict(updateBranding.error)
      ? 'Branding changed elsewhere. The latest version has been loaded; review it and publish again.'
      : getErrorMessage(updateBranding.error)
    : '';
  const previewPrimary = /^#[0-9a-fA-F]{6}$/.test(form.primaryColor.trim()) ? form.primaryColor.trim() : '#0ea5e9';
  const previewAccent = /^#[0-9a-fA-F]{6}$/.test(form.accentColor.trim()) ? form.accentColor.trim() : '#a855f7';
  const previewFont = getBrandingFontStack(form.font) || 'Inter, system-ui, sans-serif';

  return (
    <div className="space-y-8">
      <header>
        <div className="mb-2 flex items-center gap-2 text-sm font-medium text-primary-400"><Paintbrush className="h-4 w-4" />Workspace settings</div>
        <h1 className="text-3xl font-bold tracking-tight text-white">Restaurant branding</h1>
        <p className="mt-2 max-w-2xl text-dark-400">Publish safe visual tokens for the authenticated workspace. Public platform pages and administration remain platform-branded.</p>
      </header>
      <WorkspaceSettingsNav />
      {!canWrite && <Alert variant="info" className="flex items-start gap-2 p-4"><LockKeyhole className="mt-0.5 h-4 w-4 shrink-0" /><span><strong className="font-semibold">Read-only access.</strong> Owners and admins can publish branding.</span></Alert>}
      {success && <Alert variant="success" className="flex items-start gap-2"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />{success}</Alert>}
      {mutationError && <Alert className="flex items-start gap-2"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />{mutationError}</Alert>}

      <section className="overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/60">
        <div className="border-b border-dark-800 px-5 py-4 sm:px-6"><h2 className="font-semibold text-white">Theme tokens</h2><p className="mt-0.5 text-xs text-dark-500">Published version {version}. Empty fields inherit platform defaults.</p></div>
        {isRootTenant ? (
          <div className="px-6 py-12 text-center"><LockKeyhole className="mx-auto mb-4 h-9 w-9 text-dark-500" /><h3 className="font-semibold text-white">Platform branding is managed separately</h3><p className="mx-auto mt-2 max-w-md text-sm text-dark-400">Use the platform administration branding page for public and root-tenant presentation.</p></div>
        ) : !tenantId || brandingQuery.isPending ? (
          <div className="space-y-5 p-5 sm:p-6" aria-label="Loading restaurant branding">{[1, 2, 3].map((item) => <div key={item} className="h-16 animate-pulse rounded-xl bg-dark-800/70" />)}</div>
        ) : brandingQuery.isError ? (
          <div className="flex flex-col items-center px-6 py-16 text-center"><AlertCircle className="mb-4 h-9 w-9 text-red-400" /><h3 className="font-semibold text-white">Could not load restaurant branding</h3><p className="mt-2 max-w-md text-sm text-dark-400">{getErrorMessage(brandingQuery.error)}</p><Button variant="secondary" className="mt-5 inline-flex items-center gap-2" onClick={() => brandingQuery.refetch()}><RefreshCw className="h-4 w-4" />Retry</Button></div>
        ) : (
          <form className="grid gap-8 p-5 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.8fr)] sm:p-6" onSubmit={handleSubmit} noValidate>
            <div className="space-y-5">
              <div className="grid gap-5 sm:grid-cols-2">
                <Input label="Primary color" value={form.primaryColor} onChange={(event) => setField('primaryColor', event.target.value)} error={errors.primaryColor} placeholder="#0ea5e9" maxLength={7} disabled={!canWrite || pending} />
                <Input label="Accent color" value={form.accentColor} onChange={(event) => setField('accentColor', event.target.value)} error={errors.accentColor} placeholder="#a855f7" maxLength={7} disabled={!canWrite || pending} />
              </div>
              <div><label htmlFor="branding-font" className="mb-1 block text-sm font-medium text-dark-300">Font style</label><select id="branding-font" value={form.font} onChange={(event) => setField('font', event.target.value)} disabled={!canWrite || pending} className="w-full rounded-lg border border-dark-700 bg-dark-800 px-3 py-2 text-sm text-white focus:border-primary-500 focus:outline-none disabled:cursor-not-allowed disabled:opacity-50">{BRANDING_FONTS.map((font) => <option key={font.value || 'default'} value={font.value}>{font.label}</option>)}</select>{errors.font && <p className="mt-1 text-xs text-red-400">{errors.font}</p>}</div>
              {canWrite && <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end"><Button type="button" variant="secondary" disabled={pending} onClick={resetToPlatform} className="inline-flex items-center justify-center gap-2"><RotateCcw className="h-4 w-4" />Reset to platform</Button><Button type="submit" disabled={pending} className="inline-flex items-center justify-center gap-2">{pending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}{pending ? 'Publishing...' : 'Publish branding'}</Button></div>}
            </div>
            <div><p className="mb-2 text-xs font-semibold uppercase tracking-[0.16em] text-dark-500">Live preview</p><div className="overflow-hidden rounded-2xl border border-dark-700 bg-dark-950 shadow-2xl" style={{ fontFamily: previewFont }}><div className="h-2" style={{ background: `linear-gradient(90deg, ${previewPrimary}, ${previewAccent})` }} /><div className="p-6"><div className="mb-6 flex items-center gap-3"><div className="flex h-10 w-10 items-center justify-center rounded-xl text-lg font-bold text-white" style={{ backgroundColor: previewPrimary }}>R</div><div><p className="font-semibold text-white">{activeTenant?.tenantName ?? 'Your restaurant'}</p><p className="text-xs text-dark-500">Workspace preview</p></div></div><p className="text-sm text-dark-400">A focused operational workspace with your approved colors and font style.</p><div className="mt-5 inline-flex rounded-lg px-4 py-2 text-sm font-semibold text-white" style={{ backgroundColor: previewPrimary }}>Primary action</div><span className="ml-3 text-sm font-medium" style={{ color: previewAccent }}>Accent detail</span></div></div></div>
          </form>
        )}
      </section>
    </div>
  );
}
