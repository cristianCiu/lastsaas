import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { AlertCircle, CheckCircle2, ImageUp, LockKeyhole, Paintbrush, RefreshCw, RotateCcw, Save, Trash2 } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import WorkspaceSettingsNav from '../../components/WorkspaceSettingsNav';
import { useAuth } from '../../contexts/AuthContext';
import { useTenantBranding } from '../../contexts/TenantBrandingContext';
import { useTenant } from '../../contexts/TenantContext';
import { getErrorMessage } from '../../utils/errors';
import { tenantBrandingApi } from './api';
import { tenantBrandingKeys } from './queries';
import type { TenantBranding, TenantBrandingAsset, TenantBrandingAssetKind, UpdateTenantBrandingInput } from './types';
import { BRANDING_FONTS, getBrandingFontStack, normalizeTenantBranding, validateTenantBranding, type TenantBrandingFields, type TenantBrandingValidationErrors } from './validation';

const DEFAULT_FORM: TenantBrandingFields = { primaryColor: '', accentColor: '', font: '' };
const MAX_LOGO_SIZE = 900 * 1024;

function isVersionConflict(error: unknown) {
  return axios.isAxiosError(error) && error.response?.data?.code === 'VERSION_CONFLICT';
}

function LogoAssetCard({ kind, title, description, asset, logoUrl, canWrite, pending, onUpload, onDelete }: {
  kind: TenantBrandingAssetKind;
  title: string;
  description: string;
  asset?: TenantBrandingAsset;
  logoUrl: string | null;
  canWrite: boolean;
  pending: boolean;
  onUpload: (kind: TenantBrandingAssetKind, file: File, version: number) => void;
  onDelete: (kind: TenantBrandingAssetKind, version: number) => void;
}) {
  const inputId = `tenant-${kind}-logo`;
  return (
    <div className="flex min-w-0 flex-col gap-4 rounded-xl border border-dark-800 bg-dark-950/60 p-4 sm:flex-row sm:items-center">
      <div className={`flex shrink-0 items-center justify-center overflow-hidden border border-dark-700 bg-dark-900 ${kind === 'compact' ? 'h-20 w-20 rounded-2xl' : 'h-20 w-full rounded-xl sm:w-44'}`}>
        {logoUrl ? <img src={logoUrl} alt={`${title} preview`} className="h-full w-full object-contain p-2" /> : <ImageUp className="h-7 w-7 text-dark-600" />}
      </div>
      <div className="min-w-0 flex-1">
        <h3 className="font-medium text-white">{title}</h3>
        <p className="mt-1 text-xs text-dark-400">{description}</p>
        {asset && <p className="mt-2 text-xs text-dark-500">{asset.width}×{asset.height}px · {Math.ceil(asset.size / 1024)} KiB · version {asset.version}</p>}
      </div>
      {canWrite && (
        <div className="flex shrink-0 gap-2">
          <input
            id={inputId}
            aria-label={title}
            type="file"
            accept="image/png,image/jpeg"
            className="sr-only"
            disabled={pending}
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = '';
              if (file) onUpload(kind, file, asset?.version ?? 0);
            }}
          />
          <label htmlFor={inputId} className={`inline-flex cursor-pointer items-center gap-1.5 rounded-lg border border-dark-700 bg-dark-800 px-3 py-1.5 text-xs font-medium text-dark-300 transition-colors hover:text-white ${pending ? 'pointer-events-none opacity-50' : ''}`}>
            {pending ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <ImageUp className="h-3.5 w-3.5" />}{asset ? 'Replace' : 'Upload'}
          </label>
          {asset && <Button type="button" variant="danger" size="sm" disabled={pending} onClick={() => onDelete(kind, asset.version)} className="inline-flex items-center gap-1.5"><Trash2 className="h-3.5 w-3.5" />Remove</Button>}
        </div>
      )}
    </div>
  );
}

export default function TenantBrandingPage() {
  const { user } = useAuth();
  const { activeTenant, role, isRootTenant } = useTenant();
  const { assets, assetsLoading, assetsError, primaryLogoUrl, compactLogoUrl } = useTenantBranding();
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
  const [assetMessage, setAssetMessage] = useState('');
  const [assetValidationError, setAssetValidationError] = useState('');

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
  const uploadAsset = useMutation({
    mutationFn: ({ kind, file, version }: { principalId: string; tenantId: string; kind: TenantBrandingAssetKind; file: File; version: number }) => tenantBrandingApi.uploadAsset(kind, file, version),
    onSuccess: ({ asset }, variables) => {
      const assetsKey = tenantBrandingKeys.assets(variables.principalId, variables.tenantId);
      queryClient.setQueryData<{ assets: TenantBrandingAsset[] }>(assetsKey, (current) => ({ assets: [...(current?.assets ?? []).filter((item) => item.kind !== asset.kind), asset] }));
      queryClient.removeQueries({ queryKey: tenantBrandingKeys.assetKind(variables.principalId, variables.tenantId, variables.kind) });
      if (scope.current === `${variables.principalId}:${variables.tenantId}`) setAssetMessage(`${variables.kind === 'primary' ? 'Primary' : 'Compact'} logo uploaded.`);
    },
    onError: async (error, variables) => {
      if (isVersionConflict(error)) await queryClient.invalidateQueries({ queryKey: tenantBrandingKeys.assets(variables.principalId, variables.tenantId), exact: true });
    },
  });
  const deleteAsset = useMutation({
    mutationFn: ({ kind, version }: { principalId: string; tenantId: string; kind: TenantBrandingAssetKind; version: number }) => tenantBrandingApi.deleteAsset(kind, version),
    onSuccess: (_, variables) => {
      const assetsKey = tenantBrandingKeys.assets(variables.principalId, variables.tenantId);
      queryClient.setQueryData<{ assets: TenantBrandingAsset[] }>(assetsKey, (current) => ({ assets: (current?.assets ?? []).filter((item) => item.kind !== variables.kind) }));
      queryClient.removeQueries({ queryKey: tenantBrandingKeys.assetKind(variables.principalId, variables.tenantId, variables.kind) });
      if (scope.current === `${variables.principalId}:${variables.tenantId}`) setAssetMessage(`${variables.kind === 'primary' ? 'Primary' : 'Compact'} logo removed.`);
    },
    onError: async (error, variables) => {
      if (isVersionConflict(error)) await queryClient.invalidateQueries({ queryKey: tenantBrandingKeys.assets(variables.principalId, variables.tenantId), exact: true });
    },
  });

  const resetMutation = updateBranding.reset;
  useLayoutEffect(() => {
    scope.current = scopeKey;
    setForm(DEFAULT_FORM);
    setVersion(0);
    setErrors({});
    setSuccess('');
    setAssetMessage('');
    setAssetValidationError('');
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
  const handleLogoUpload = (kind: TenantBrandingAssetKind, file: File, assetVersion: number) => {
    setAssetMessage('');
    setAssetValidationError('');
    uploadAsset.reset();
    deleteAsset.reset();
    if (file.type !== 'image/png' && file.type !== 'image/jpeg') {
      setAssetValidationError('Choose a PNG or JPEG image. SVG and animated formats are not accepted.');
      return;
    }
    if (!file.size || file.size > MAX_LOGO_SIZE) {
      setAssetValidationError('Logo files must be smaller than 900 KiB.');
      return;
    }
    uploadAsset.mutate({ principalId, tenantId, kind, file, version: assetVersion });
  };
  const handleLogoDelete = (kind: TenantBrandingAssetKind, assetVersion: number) => {
    setAssetMessage('');
    setAssetValidationError('');
    uploadAsset.reset();
    deleteAsset.reset();
    deleteAsset.mutate({ principalId, tenantId, kind, version: assetVersion });
  };

  const mutationInScope = updateBranding.variables?.principalId === principalId && updateBranding.variables?.tenantId === tenantId;
  const pending = mutationInScope && updateBranding.isPending;
  const mutationError = mutationInScope && updateBranding.isError
    ? isVersionConflict(updateBranding.error)
      ? 'Branding changed elsewhere. The latest version has been loaded; review it and publish again.'
      : getErrorMessage(updateBranding.error)
    : '';
  const assetMutation = uploadAsset.isError ? uploadAsset : deleteAsset;
  const assetMutationInScope = assetMutation.variables?.principalId === principalId && assetMutation.variables?.tenantId === tenantId;
  const assetMutationError = assetMutationInScope && assetMutation.isError
    ? isVersionConflict(assetMutation.error)
      ? 'This logo changed elsewhere. The latest asset version has been loaded; try again.'
      : getErrorMessage(assetMutation.error)
    : '';
  const uploadInScope = uploadAsset.variables?.principalId === principalId && uploadAsset.variables?.tenantId === tenantId;
  const deleteInScope = deleteAsset.variables?.principalId === principalId && deleteAsset.variables?.tenantId === tenantId;
  const pendingKind = uploadAsset.isPending && uploadInScope ? uploadAsset.variables?.kind : deleteAsset.isPending && deleteInScope ? deleteAsset.variables?.kind : undefined;
  const primaryAsset = assets.find((asset) => asset.kind === 'primary');
  const compactAsset = assets.find((asset) => asset.kind === 'compact');
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
      {assetMessage && <Alert variant="success" className="flex items-start gap-2"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />{assetMessage}</Alert>}
      {(assetValidationError || assetMutationError) && <Alert className="flex items-start gap-2"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />{assetValidationError || assetMutationError}</Alert>}

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

      {!isRootTenant && tenantId && (
        <section className="overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/60">
          <div className="border-b border-dark-800 px-5 py-4 sm:px-6"><h2 className="font-semibold text-white">Restaurant logos</h2><p className="mt-0.5 text-xs text-dark-500">Private PNG or JPEG files, up to 900 KiB and 2048×2048 pixels. Public platform pages keep the platform logo.</p></div>
          {assetsLoading ? <div className="space-y-4 p-5 sm:p-6" aria-label="Loading restaurant logos"><div className="h-28 animate-pulse rounded-xl bg-dark-800/70" /><div className="h-28 animate-pulse rounded-xl bg-dark-800/70" /></div> : assetsError ? <div className="px-6 py-10 text-center text-sm text-red-400">{getErrorMessage(assetsError)}</div> : <div className="space-y-4 p-5 sm:p-6"><LogoAssetCard kind="primary" title="Primary logo" description="Horizontal logo for the desktop workspace header." asset={primaryAsset} logoUrl={primaryLogoUrl} canWrite={canWrite} pending={pendingKind === 'primary'} onUpload={handleLogoUpload} onDelete={handleLogoDelete} /><LogoAssetCard kind="compact" title="Compact logo" description="Square logo for narrow layouts and compact identity slots." asset={compactAsset} logoUrl={compactLogoUrl} canWrite={canWrite} pending={pendingKind === 'compact'} onUpload={handleLogoUpload} onDelete={handleLogoDelete} /></div>}
        </section>
      )}
    </div>
  );
}
