import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { AlertCircle, CheckCircle2, Clock3, LockKeyhole, MapPin, Pencil, Plus, Power, RefreshCw, X } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import TableSkeleton from '../../components/TableSkeleton';
import { useTenant } from '../../contexts/TenantContext';
import { useActiveLocation } from '../../contexts/ActiveLocationContext';
import { getErrorMessage } from '../../utils/errors';
import { locationsApi } from './api';
import { locationKeys } from './queries';
import type { CreateLocationInput, Location, UpdateLocationInput } from './types';
import { getTimezoneOptions, validateLocation, type LocationValidationErrors } from './validation';

const EMPTY_FORM: CreateLocationInput = { code: '', name: '', timezone: '' };
const TIMEZONE_OPTIONS = getTimezoneOptions();

function isAuthorizationError(error: unknown) {
  return axios.isAxiosError(error) && (error.response?.status === 401 || error.response?.status === 403);
}

function isVersionConflict(error: unknown) {
  if (!axios.isAxiosError(error)) return false;
  const data = error.response?.data;
  return !!data && typeof data === 'object' && 'code' in data && data.code === 'VERSION_CONFLICT';
}

interface EditForm {
  name: string;
  timezone: string;
}

export default function LocationsPage() {
  const { activeTenant, role } = useTenant();
  const tenantId = activeTenant?.tenantId ?? '';
  const canCreate = role === 'owner' || role === 'admin';
  const { locations, loading: locationsLoading, error: locationsError } = useActiveLocation();
  const queryClient = useQueryClient();
  const activeTenantId = useRef(tenantId);
  const [form, setForm] = useState<CreateLocationInput>(EMPTY_FORM);
  const [errors, setErrors] = useState<LocationValidationErrors>({});
  const [success, setSuccess] = useState('');
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<EditForm>({ name: '', timezone: '' });
  const [editErrors, setEditErrors] = useState<Pick<LocationValidationErrors, 'name' | 'timezone'>>({});

  useEffect(() => {
    activeTenantId.current = tenantId;
    setSuccess('');
    setEditingId(null);
  }, [tenantId]);

  const createLocation = useMutation({
    mutationFn: ({ input }: { input: CreateLocationInput; tenantId: string }) => locationsApi.create(input),
    onSuccess: async ({ location }, variables) => {
      await queryClient.invalidateQueries({ queryKey: locationKeys.list(variables.tenantId), exact: true });
      if (activeTenantId.current !== variables.tenantId) return;

      setForm(EMPTY_FORM);
      setErrors({});
      setSuccess(`${location.name} was created successfully.`);
    },
  });

  const updateLocation = useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateLocationInput; tenantId: string }) =>
      locationsApi.update(id, input),
    onMutate: () => {
      setSuccess('');
    },
    onSuccess: async ({ location }, variables) => {
      queryClient.setQueryData<{ locations: Location[] }>(locationKeys.list(variables.tenantId), (current) => current ? {
        locations: current.locations.map((item) => item.id === location.id ? location : item),
      } : current);
      await queryClient.invalidateQueries({ queryKey: locationKeys.list(variables.tenantId), exact: true });
      if (activeTenantId.current !== variables.tenantId) return;
      setEditingId(null);
      setEditErrors({});
      setSuccess(`${location.name} was updated successfully.`);
    },
    onError: async (_error, variables) => {
      if (isVersionConflict(_error)) {
        await queryClient.invalidateQueries({ queryKey: locationKeys.list(variables.tenantId), exact: true });
        if (activeTenantId.current === variables.tenantId) setEditingId(null);
      }
    },
  });

  const setField = (field: keyof CreateLocationInput, value: string) => {
    setForm((current) => ({ ...current, [field]: value }));
    setErrors((current) => ({ ...current, [field]: undefined }));
    setSuccess('');
    createLocation.reset();
  };

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!canCreate) return;

    const validationErrors = validateLocation(form);
    setErrors(validationErrors);
    setSuccess('');
    if (Object.keys(validationErrors).length > 0) return;

    createLocation.mutate({
      tenantId,
      input: {
        code: form.code.trim(),
        name: form.name.trim(),
        timezone: form.timezone.trim(),
      },
    });
  };

  const startEditing = (location: Location) => {
    setEditingId(location.id);
    setEditForm({ name: location.name, timezone: location.timezone });
    setEditErrors({});
    setSuccess('');
    updateLocation.reset();
  };

  const saveEdit = (location: Location) => {
    const validationErrors = validateLocation({ code: location.code, ...editForm });
    const nextErrors = { name: validationErrors.name, timezone: validationErrors.timezone };
    setEditErrors(nextErrors);
    if (nextErrors.name || nextErrors.timezone) return;

    updateLocation.mutate({
      id: location.id,
      tenantId,
      input: { version: location.version, name: editForm.name.trim(), timezone: editForm.timezone.trim() },
    });
  };

  const toggleLocation = (location: Location) => {
    if (location.isActive && !window.confirm(`Deactivate ${location.name}? It will no longer be available for active selection.`)) return;
    updateLocation.mutate({
      id: location.id,
      tenantId,
      input: { version: location.version, isActive: !location.isActive },
    });
  };

  const unauthorized = !!locationsError && isAuthorizationError(locationsError);
  const updateError = updateLocation.isError
    ? isVersionConflict(updateLocation.error)
      ? 'This location changed elsewhere. The latest data has been loaded; review it and try again.'
      : getErrorMessage(updateLocation.error)
    : '';

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="mb-2 flex items-center gap-2 text-sm font-medium text-primary-400">
            <MapPin className="h-4 w-4" />
            Workspace settings
          </div>
          <h1 className="text-3xl font-bold tracking-tight text-white">Locations</h1>
          <p className="mt-2 max-w-2xl text-dark-400">
            Define the places your team operates and keep local times consistent across the workspace.
          </p>
        </div>
        {!locationsLoading && !locationsError && (
          <div className="flex items-center gap-2 self-start rounded-full border border-dark-700 bg-dark-900 px-3 py-1.5 text-xs text-dark-300 sm:self-auto">
            <span className="h-2 w-2 rounded-full bg-accent-emerald" />
            {locations.length} {locations.length === 1 ? 'location' : 'locations'}
          </div>
        )}
      </header>

      {!canCreate && !unauthorized && (
        <Alert variant="info" className="flex items-start gap-2 p-4">
          <LockKeyhole className="mt-0.5 h-4 w-4 shrink-0" />
          <span><strong className="font-semibold">Read-only access.</strong> Owners and admins can add, edit, and activate locations.</span>
        </Alert>
      )}

      {success && <Alert variant="success" className="flex items-start gap-2"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />{success}</Alert>}
      {updateError && <Alert className="flex items-start gap-2"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />{updateError}</Alert>}

      <div className={`grid gap-6 ${canCreate ? 'lg:grid-cols-[minmax(0,1fr)_22rem]' : ''}`}>
        <section className="min-w-0 overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/50 backdrop-blur-sm">
          <div className="flex items-center justify-between border-b border-dark-800 px-5 py-4 sm:px-6">
            <div>
              <h2 className="font-semibold text-white">Workspace locations</h2>
              <p className="mt-0.5 text-xs text-dark-500">Scoped to {activeTenant?.tenantName ?? 'your active workspace'}</p>
            </div>
          </div>

          {!tenantId || locationsLoading ? (
            <TableSkeleton rows={3} cols={3} />
          ) : unauthorized ? (
            <div className="flex flex-col items-center px-6 py-16 text-center">
              <LockKeyhole className="mb-4 h-9 w-9 text-red-400" />
              <h3 className="font-semibold text-white">Location access unavailable</h3>
              <p className="mt-2 max-w-md text-sm text-dark-400">You are not authorized to view locations for this workspace.</p>
            </div>
          ) : locationsError ? (
            <div className="flex flex-col items-center px-6 py-16 text-center">
              <AlertCircle className="mb-4 h-9 w-9 text-red-400" />
              <h3 className="font-semibold text-white">Could not load locations</h3>
              <p className="mt-2 max-w-md text-sm text-dark-400">{getErrorMessage(locationsError)}</p>
              <Button className="mt-5 inline-flex items-center gap-2" variant="secondary" onClick={() => queryClient.refetchQueries({ queryKey: locationKeys.list(tenantId), exact: true })}>
                <RefreshCw className="h-4 w-4" /> Retry
              </Button>
            </div>
          ) : locations.length === 0 ? (
            <div className="flex flex-col items-center px-6 py-16 text-center">
              <div className="mb-4 rounded-2xl border border-dark-700 bg-dark-800 p-3">
                <MapPin className="h-7 w-7 text-primary-400" />
              </div>
              <h3 className="font-semibold text-white">No locations yet</h3>
              <p className="mt-2 max-w-sm text-sm text-dark-400">
                {canCreate ? 'Add your first operating location using the form.' : 'An owner or admin has not added any locations.'}
              </p>
            </div>
          ) : (
            <ul className="divide-y divide-dark-800">
              {locations.map((location) => (
                <li key={location.id} className="group px-5 py-5 transition-colors hover:bg-dark-800/30 sm:px-6">
                  {editingId === location.id ? (
                    <div className="space-y-4">
                      <div className="grid gap-4 sm:grid-cols-2">
                        <Input
                          label="Name"
                          value={editForm.name}
                          onChange={(event) => {
                            setEditForm((current) => ({ ...current, name: event.target.value }));
                            setEditErrors((current) => ({ ...current, name: undefined }));
                            updateLocation.reset();
                          }}
                          error={editErrors.name}
                          disabled={updateLocation.isPending}
                        />
                        <Input
                          label="IANA timezone"
                          value={editForm.timezone}
                          onChange={(event) => {
                            setEditForm((current) => ({ ...current, timezone: event.target.value }));
                            setEditErrors((current) => ({ ...current, timezone: undefined }));
                            updateLocation.reset();
                          }}
                          error={editErrors.timezone}
                          list="location-timezones"
                          disabled={updateLocation.isPending}
                        />
                      </div>
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <code className="text-xs text-dark-500">{location.code}</code>
                        <div className="flex gap-2">
                          <Button variant="ghost" size="sm" onClick={() => setEditingId(null)} disabled={updateLocation.isPending} className="inline-flex items-center gap-1.5">
                            <X className="h-3.5 w-3.5" /> Cancel
                          </Button>
                          <Button size="sm" onClick={() => saveEdit(location)} disabled={updateLocation.isPending} className="inline-flex items-center gap-1.5">
                            {updateLocation.isPending ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
                            {updateLocation.isPending ? 'Saving...' : 'Save changes'}
                          </Button>
                        </div>
                      </div>
                    </div>
                  ) : (
                  <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                    <div className="flex min-w-0 items-center gap-4">
                      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-500/10 text-primary-400">
                        <MapPin className="h-5 w-5" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="truncate font-medium text-white">{location.name}</h3>
                          <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${location.isActive ? 'bg-accent-emerald/10 text-accent-emerald' : 'bg-dark-700 text-dark-400'}`}>
                            {location.isActive ? 'Active' : 'Inactive'}
                          </span>
                        </div>
                        <code className="mt-1 block text-xs text-dark-500">{location.code}</code>
                      </div>
                    </div>
                    <div className="flex flex-wrap items-center gap-3 pl-14 sm:justify-end sm:pl-0">
                      <div className="flex items-center gap-2 text-sm text-dark-400">
                        <Clock3 className="h-4 w-4 text-dark-500" />
                        <span>{location.timezone.replaceAll('_', ' ')}</span>
                      </div>
                      {canCreate && (
                        <div className="flex gap-2">
                          <Button variant="secondary" size="sm" onClick={() => startEditing(location)} disabled={updateLocation.isPending} className="inline-flex items-center gap-1.5">
                            <Pencil className="h-3.5 w-3.5" /> Edit
                          </Button>
                          <Button variant={location.isActive ? 'danger' : 'secondary'} size="sm" onClick={() => toggleLocation(location)} disabled={updateLocation.isPending} className="inline-flex items-center gap-1.5">
                            {updateLocation.isPending && updateLocation.variables?.id === location.id ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Power className="h-3.5 w-3.5" />}
                            {location.isActive ? 'Deactivate' : 'Reactivate'}
                          </Button>
                        </div>
                      )}
                    </div>
                  </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </section>

        {canCreate && (
          <aside className="h-fit rounded-2xl border border-dark-800 bg-dark-900/70 p-5 sm:p-6 lg:sticky lg:top-24">
            <div className="mb-5 flex items-center gap-3">
              <div className="rounded-lg bg-primary-500/10 p-2 text-primary-400"><Plus className="h-5 w-5" /></div>
              <div>
                <h2 className="font-semibold text-white">Add a location</h2>
                <p className="text-xs text-dark-500">Create a new operating place</p>
              </div>
            </div>

            {createLocation.isError && <Alert className="mb-4">{getErrorMessage(createLocation.error)}</Alert>}

            <form className="space-y-4" onSubmit={handleSubmit} noValidate>
              <Input
                label="Code"
                value={form.code}
                onChange={(event) => setField('code', event.target.value)}
                error={errors.code}
                placeholder="new-york"
                autoComplete="off"
                disabled={createLocation.isPending}
                aria-describedby="location-code-hint"
              />
              {!errors.code && <p id="location-code-hint" className="-mt-2 text-xs text-dark-500">Lower-case slug used in references.</p>}
              <Input
                label="Name"
                value={form.name}
                onChange={(event) => setField('name', event.target.value)}
                error={errors.name}
                placeholder="New York"
                disabled={createLocation.isPending}
              />
              <div>
                <Input
                  label="IANA timezone"
                  value={form.timezone}
                  onChange={(event) => setField('timezone', event.target.value)}
                  error={errors.timezone}
                  placeholder="America/New_York"
                  list="location-timezones"
                  autoComplete="off"
                  disabled={createLocation.isPending}
                />
                <datalist id="location-timezones">
                  {TIMEZONE_OPTIONS.map((timezone) => <option key={timezone} value={timezone} />)}
                </datalist>
              </div>
              <Button type="submit" className="flex w-full items-center justify-center gap-2 py-2.5" disabled={createLocation.isPending}>
                {createLocation.isPending ? <><RefreshCw className="h-4 w-4 animate-spin" /> Creating...</> : <><Plus className="h-4 w-4" /> Create location</>}
              </Button>
            </form>
          </aside>
        )}
      </div>
    </div>
  );
}
