import { useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { AlertCircle, Archive, CheckCircle2, LockKeyhole, MapPin, Pencil, Plus, Power, RefreshCw, Snowflake, X } from 'lucide-react';
import ConfirmModal from '../../components/ConfirmModal';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import TableSkeleton from '../../components/TableSkeleton';
import WorkspaceSettingsNav from '../../components/WorkspaceSettingsNav';
import { useAuth } from '../../contexts/AuthContext';
import { useActiveLocation } from '../../contexts/ActiveLocationContext';
import { useStaffProfile } from '../../contexts/StaffProfileContext';
import { useTenant } from '../../contexts/TenantContext';
import { getErrorMessage } from '../../utils/errors';
import { storageAreasApi } from './api';
import { storageAreaKeys } from './queries';
import { STORAGE_AREA_TYPES, type CreateStorageAreaInput, type StorageArea, type StorageAreaType, type UpdateStorageAreaInput } from './types';
import { validateStorageArea, type StorageAreaValidationErrors } from './validation';

const EMPTY_FORM: CreateStorageAreaInput = { name: '', type: 'dry' };
const TYPE_LABELS: Record<StorageAreaType, string> = { refrigerated: 'Refrigerated', frozen: 'Frozen', bar: 'Bar', dry: 'Dry storage', other: 'Other' };

function isVersionConflict(error: unknown) {
  return axios.isAxiosError(error) && error.response?.data?.code === 'VERSION_CONFLICT';
}

export default function StorageAreasPage() {
  const { user } = useAuth();
  const { activeTenant } = useTenant();
  const { profile, loading: profileLoading, error: profileError, missing: profileMissing, hasPermission } = useStaffProfile();
  const { activeLocation, loading: locationsLoading, error: locationsError } = useActiveLocation();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const locationId = activeLocation?.id ?? '';
  const canRead = hasPermission('storage_areas.read');
  const canManage = hasPermission('storage_areas.manage');
  const scopeRef = useRef({ principalId, tenantId, locationId });
  const queryClient = useQueryClient();
  const [form, setForm] = useState<CreateStorageAreaInput>(EMPTY_FORM);
  const [errors, setErrors] = useState<StorageAreaValidationErrors>({});
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<CreateStorageAreaInput>(EMPTY_FORM);
  const [editErrors, setEditErrors] = useState<StorageAreaValidationErrors>({});
  const [toggleTarget, setToggleTarget] = useState<StorageArea | null>(null);
  const [success, setSuccess] = useState('');

  const storageQuery = useQuery({
    queryKey: storageAreaKeys.list(principalId, tenantId, locationId),
    queryFn: () => storageAreasApi.list(locationId),
    enabled: canRead && !!principalId && !!tenantId && !!locationId,
  });

  const isCurrentScope = (scope: { principalId: string; tenantId: string; locationId: string }) =>
    scopeRef.current.principalId === scope.principalId && scopeRef.current.tenantId === scope.tenantId && scopeRef.current.locationId === scope.locationId;

  const createArea = useMutation({
    mutationFn: ({ locationId, input }: { principalId: string; tenantId: string; locationId: string; input: CreateStorageAreaInput }) => storageAreasApi.create(locationId, input),
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: storageAreaKeys.list(variables.principalId, variables.tenantId, variables.locationId), exact: true });
      if (isCurrentScope(variables)) setSuccess('');
    },
    onSuccess: async ({ storageArea }, variables) => {
      queryClient.setQueryData<{ storageAreas: StorageArea[] }>(storageAreaKeys.list(variables.principalId, variables.tenantId, variables.locationId), (current) => current ? {
        storageAreas: [...current.storageAreas, storageArea],
      } : { storageAreas: [storageArea] });
      await queryClient.invalidateQueries({ queryKey: storageAreaKeys.list(variables.principalId, variables.tenantId, variables.locationId), exact: true });
      if (!isCurrentScope(variables)) return;
      setForm(EMPTY_FORM);
      setErrors({});
      setSuccess(`${storageArea.name} was created.`);
    },
  });

  const updateArea = useMutation({
    mutationFn: ({ locationId, id, input }: { principalId: string; tenantId: string; locationId: string; id: string; input: UpdateStorageAreaInput }) => storageAreasApi.update(locationId, id, input),
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: storageAreaKeys.list(variables.principalId, variables.tenantId, variables.locationId), exact: true });
      if (isCurrentScope(variables)) setSuccess('');
    },
    onSuccess: async ({ storageArea }, variables) => {
      queryClient.setQueryData<{ storageAreas: StorageArea[] }>(storageAreaKeys.list(variables.principalId, variables.tenantId, variables.locationId), (current) => current ? {
        storageAreas: current.storageAreas.map((area) => area.id === storageArea.id ? storageArea : area),
      } : current);
      await queryClient.invalidateQueries({ queryKey: storageAreaKeys.list(variables.principalId, variables.tenantId, variables.locationId), exact: true });
      if (!isCurrentScope(variables)) return;
      setEditingId(null);
      setToggleTarget(null);
      setEditErrors({});
      setSuccess(`${storageArea.name} was ${storageArea.isActive ? 'updated' : 'deactivated'}.`);
    },
    onError: async (error, variables) => {
      if (!isVersionConflict(error)) return;
      await queryClient.invalidateQueries({ queryKey: storageAreaKeys.list(variables.principalId, variables.tenantId, variables.locationId), exact: true });
      if (isCurrentScope(variables)) {
        setEditingId(null);
        setToggleTarget(null);
      }
    },
  });

  const resetCreateArea = createArea.reset;
  const resetUpdateArea = updateArea.reset;

  useLayoutEffect(() => {
    scopeRef.current = { principalId, tenantId, locationId };
    setForm(EMPTY_FORM);
    setErrors({});
    setEditingId(null);
    setEditForm(EMPTY_FORM);
    setEditErrors({});
    setToggleTarget(null);
    setSuccess('');
    resetCreateArea();
    resetUpdateArea();
  }, [principalId, tenantId, locationId, resetCreateArea, resetUpdateArea]);

  const submitCreate = (event: FormEvent) => {
    event.preventDefault();
    const validationErrors = validateStorageArea(form);
    setErrors(validationErrors);
    if (!canManage || !tenantId || !locationId || Object.keys(validationErrors).length) return;
    createArea.mutate({ principalId, tenantId, locationId, input: { name: form.name.trim(), type: form.type } });
  };

  const startEditing = (area: StorageArea) => {
    setEditingId(area.id);
    setEditForm({ name: area.name, type: area.type });
    setEditErrors({});
    setSuccess('');
    updateArea.reset();
  };

  const saveEdit = (area: StorageArea) => {
    const validationErrors = validateStorageArea(editForm);
    setEditErrors(validationErrors);
    if (Object.keys(validationErrors).length) return;
    if (!canManage) return;
    updateArea.mutate({ principalId, tenantId, locationId, id: area.id, input: { version: area.version, name: editForm.name.trim(), type: editForm.type } });
  };

  const toggleArea = (area: StorageArea) => {
    if (!canManage) return;
    updateArea.mutate({ principalId, tenantId, locationId, id: area.id, input: { version: area.version, isActive: !area.isActive } });
  };

  const createInScope = createArea.variables?.principalId === principalId && createArea.variables.tenantId === tenantId && createArea.variables.locationId === locationId;
  const updateInScope = updateArea.variables?.principalId === principalId && updateArea.variables.tenantId === tenantId && updateArea.variables.locationId === locationId;
  const createPending = createInScope && createArea.isPending;
  const updatePending = updateInScope && updateArea.isPending;
  const updateError = updateInScope && updateArea.isError
    ? isVersionConflict(updateArea.error)
      ? 'This storage area changed elsewhere. The latest list has been loaded; review it and try again.'
      : getErrorMessage(updateArea.error)
    : '';
  const areas = storageQuery.data?.storageAreas ?? [];

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="mb-2 flex items-center gap-2 text-sm font-medium text-primary-400"><Archive className="h-4 w-4" />Workspace settings</div>
          <h1 className="text-3xl font-bold tracking-tight text-white">Storage areas</h1>
          <p className="mt-2 max-w-2xl text-dark-400">Organize stock by the physical areas at the currently selected location.</p>
        </div>
        {activeLocation && <div className="flex max-w-full items-center gap-3 rounded-xl border border-primary-500/20 bg-primary-500/10 px-4 py-3 sm:max-w-sm"><MapPin className="h-5 w-5 shrink-0 text-primary-400" /><div className="min-w-0"><p className="text-[11px] font-medium uppercase tracking-wider text-primary-400">Selected location</p><p className="truncate font-semibold text-white">{activeLocation.name}</p></div></div>}
      </header>

      <WorkspaceSettingsNav />

      {canRead && !canManage && <Alert variant="info" className="flex items-start gap-2 p-4"><LockKeyhole className="mt-0.5 h-4 w-4 shrink-0" /><span><strong className="font-semibold">Read-only access.</strong> You can view storage areas but cannot change them.</span></Alert>}
      {canManage && !canRead && <Alert variant="info" className="flex items-start gap-2 p-4"><LockKeyhole className="mt-0.5 h-4 w-4 shrink-0" /><span><strong className="font-semibold">Create-only visibility.</strong> You may create storage areas at the selected location, but viewing and editing the list requires storage read access.</span></Alert>}
      {success && <Alert variant="success" className="flex items-start gap-2"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />{success}</Alert>}
      {updateError && <Alert className="flex items-start gap-2"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />{updateError}</Alert>}

      {profileLoading ? (
        <section className="overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/50"><TableSkeleton rows={3} cols={3} /></section>
      ) : profileError ? (
        <section className="flex flex-col items-center rounded-2xl border border-dark-800 bg-dark-900/50 px-6 py-16 text-center"><AlertCircle className="mb-4 h-9 w-9 text-red-400" /><h2 className="font-semibold text-white">Could not load staff access</h2><p className="mt-2 max-w-md text-sm text-dark-400">{getErrorMessage(profileError)}</p></section>
      ) : profileMissing ? (
        <section className="flex flex-col items-center rounded-2xl border border-dark-800 bg-dark-900/50 px-6 py-16 text-center"><LockKeyhole className="mb-4 h-9 w-9 text-dark-500" /><h2 className="font-semibold text-white">Staff profile required</h2><p className="mt-2 max-w-md text-sm text-dark-400">Ask a workspace owner to create your staff profile and assign location access.</p></section>
      ) : profile?.status === 'inactive' ? (
        <section className="flex flex-col items-center rounded-2xl border border-dark-800 bg-dark-900/50 px-6 py-16 text-center"><LockKeyhole className="mb-4 h-9 w-9 text-dark-500" /><h2 className="font-semibold text-white">Staff profile inactive</h2><p className="mt-2 max-w-md text-sm text-dark-400">Your operational access has been deactivated. Contact a workspace owner.</p></section>
      ) : !canRead && !canManage ? (
        <section className="flex flex-col items-center rounded-2xl border border-dark-800 bg-dark-900/50 px-6 py-16 text-center"><LockKeyhole className="mb-4 h-9 w-9 text-dark-500" /><h2 className="font-semibold text-white">Storage area access unavailable</h2><p className="mt-2 max-w-md text-sm text-dark-400">Your staff profile does not grant permission to view storage areas.</p></section>
      ) : locationsLoading ? (
        <section className="overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/50"><TableSkeleton rows={3} cols={3} /></section>
      ) : locationsError ? (
        <section className="flex flex-col items-center rounded-2xl border border-dark-800 bg-dark-900/50 px-6 py-16 text-center"><AlertCircle className="mb-4 h-9 w-9 text-red-400" /><h2 className="font-semibold text-white">Could not determine the active location</h2><p className="mt-2 max-w-md text-sm text-dark-400">{getErrorMessage(locationsError)}</p></section>
      ) : !activeLocation ? (
        <section className="flex flex-col items-center rounded-2xl border border-dark-800 bg-dark-900/50 px-6 py-16 text-center"><MapPin className="mb-4 h-9 w-9 text-primary-400" /><h2 className="font-semibold text-white">Select an active location first</h2><p className="mt-2 max-w-md text-sm text-dark-400">Storage areas belong to one location. Create or activate a location, then select it from the location picker in the header.</p></section>
      ) : (
        <div className={`grid gap-6 ${canRead && canManage ? 'lg:grid-cols-[minmax(0,1fr)_22rem]' : ''}`}>
          {canRead && <section className="min-w-0 overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/50">
            <div className="border-b border-dark-800 px-5 py-4 sm:px-6"><h2 className="font-semibold text-white">Areas at {activeLocation.name}</h2><p className="mt-0.5 text-xs text-dark-500">{areas.length} {areas.length === 1 ? 'area' : 'areas'}</p></div>
            {storageQuery.isPending ? <TableSkeleton rows={3} cols={3} /> : storageQuery.isError ? (
              <div className="flex flex-col items-center px-6 py-16 text-center"><AlertCircle className="mb-4 h-9 w-9 text-red-400" /><h3 className="font-semibold text-white">Could not load storage areas</h3><p className="mt-2 max-w-md text-sm text-dark-400">{getErrorMessage(storageQuery.error)}</p><Button variant="secondary" className="mt-5 inline-flex items-center gap-2" onClick={() => storageQuery.refetch()}><RefreshCw className="h-4 w-4" />Retry</Button></div>
            ) : areas.length === 0 ? (
              <div className="flex flex-col items-center px-6 py-16 text-center"><Snowflake className="mb-4 h-9 w-9 text-primary-400" /><h3 className="font-semibold text-white">No storage areas yet</h3><p className="mt-2 max-w-sm text-sm text-dark-400">{canManage ? `Add the first area for ${activeLocation.name} using the form.` : 'No areas have been configured at this location.'}</p></div>
            ) : (
              <ul className="divide-y divide-dark-800">{areas.map((area) => <li key={area.id} className="px-5 py-5 sm:px-6">
                {editingId === area.id ? <div className="space-y-4"><div className="grid gap-4 sm:grid-cols-2"><Input id={`storage-area-edit-name-${area.id}`} label="Name" value={editForm.name} error={editErrors.name} disabled={updatePending} onChange={(event) => { setEditForm((current) => ({ ...current, name: event.target.value })); setEditErrors((current) => ({ ...current, name: undefined })); updateArea.reset(); }} /><Select id={`storage-area-edit-type-${area.id}`} label="Type" value={editForm.type} error={editErrors.type} disabled={updatePending} onChange={(event) => setEditForm((current) => ({ ...current, type: event.target.value as StorageAreaType }))}>{STORAGE_AREA_TYPES.map((type) => <option key={type} value={type}>{TYPE_LABELS[type]}</option>)}</Select></div><div className="flex justify-end gap-2"><Button variant="ghost" size="sm" onClick={() => setEditingId(null)} disabled={updatePending} className="inline-flex items-center gap-1.5"><X className="h-3.5 w-3.5" />Cancel</Button><Button size="sm" onClick={() => saveEdit(area)} disabled={updatePending} className="inline-flex items-center gap-1.5">{updatePending ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}Save changes</Button></div></div> :
                <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h3 className="font-medium text-white">{area.name}</h3><span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${area.isActive ? 'bg-accent-emerald/10 text-accent-emerald' : 'bg-dark-700 text-dark-400'}`}>{area.isActive ? 'Active' : 'Inactive'}</span></div><p className="mt-1 text-sm text-dark-400">{TYPE_LABELS[area.type]}</p></div>{canManage && <div className="flex gap-2"><Button variant="secondary" size="sm" onClick={() => startEditing(area)} disabled={updatePending || !area.isActive} className="inline-flex items-center gap-1.5"><Pencil className="h-3.5 w-3.5" />Edit</Button><Button variant={area.isActive ? 'danger' : 'secondary'} size="sm" onClick={() => area.isActive ? setToggleTarget(area) : toggleArea(area)} disabled={updatePending} className="inline-flex items-center gap-1.5">{updatePending && updateArea.variables?.id === area.id ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Power className="h-3.5 w-3.5" />}{area.isActive ? 'Deactivate' : 'Reactivate'}</Button></div>}</div>}
              </li>)}</ul>
            )}
          </section>}
          {canManage && <aside className="h-fit rounded-2xl border border-dark-800 bg-dark-900/70 p-5 sm:p-6 lg:sticky lg:top-24"><div className="mb-5 flex items-center gap-3"><div className="rounded-lg bg-primary-500/10 p-2 text-primary-400"><Plus className="h-5 w-5" /></div><div><h2 className="font-semibold text-white">Add storage area</h2><p className="text-xs text-dark-500">At {activeLocation.name}</p></div></div>{createInScope && createArea.isError && <Alert className="mb-4">{getErrorMessage(createArea.error)}</Alert>}<form className="space-y-4" onSubmit={submitCreate} noValidate><Input id="storage-area-create-name" label="Name" value={form.name} error={errors.name} placeholder="Main cold room" disabled={createPending} onChange={(event) => { setForm((current) => ({ ...current, name: event.target.value })); setErrors((current) => ({ ...current, name: undefined })); createArea.reset(); }} /><Select id="storage-area-create-type" label="Type" value={form.type} error={errors.type} disabled={createPending} onChange={(event) => setForm((current) => ({ ...current, type: event.target.value as StorageAreaType }))}>{STORAGE_AREA_TYPES.map((type) => <option key={type} value={type}>{TYPE_LABELS[type]}</option>)}</Select><Button type="submit" className="flex w-full items-center justify-center gap-2 py-2.5" disabled={createPending}>{createPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}{createPending ? 'Creating...' : 'Create storage area'}</Button></form></aside>}
        </div>
      )}

      <ConfirmModal open={!!toggleTarget} onClose={() => !updatePending && setToggleTarget(null)} onConfirm={() => toggleTarget && toggleArea(toggleTarget)} title="Deactivate storage area?" message={`${toggleTarget?.name ?? 'This storage area'} will remain in the location history but will no longer be active.`} confirmLabel="Deactivate" confirmVariant="danger" loading={updatePending} />
    </div>
  );
}
