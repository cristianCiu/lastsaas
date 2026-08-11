import { useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { AlertCircle, CheckCircle2, Database, Edit3, Plus, RefreshCw, Ruler, Save } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import WorkspaceSettingsNav from '../../components/WorkspaceSettingsNav';
import { useAuth } from '../../contexts/AuthContext';
import { useStaffProfile } from '../../contexts/StaffProfileContext';
import { useTenant } from '../../contexts/TenantContext';
import { getErrorMessage } from '../../utils/errors';
import { unitsApi } from './api';
import { masterDataKeys } from './queries';
import type { CreateUnitInput, Unit, UnitDimension } from './types';
import { normalizeUnit, validateUnit, type UnitValidationErrors } from './validation';

const EMPTY: CreateUnitInput = { code: '', name: '', symbol: '', dimension: 'mass', precision: 3 };
const DIMENSIONS: Array<{ value: UnitDimension; label: string }> = [{ value: 'mass', label: 'Mass' }, { value: 'volume', label: 'Volume' }, { value: 'count', label: 'Count' }];

function isVersionConflict(error: unknown) {
  return axios.isAxiosError(error) && error.response?.data?.code === 'VERSION_CONFLICT';
}

export default function UnitsPage() {
  const { user } = useAuth();
  const { activeTenant, isRootTenant } = useTenant();
  const { loading: profileLoading, error: profileError, missing, hasPermission } = useStaffProfile();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const scopeKey = `${principalId}:${tenantId}`;
  const scope = useRef(scopeKey);
  const queryClient = useQueryClient();
  const canRead = !isRootTenant && hasPermission('catalog.read');
  const canManage = !isRootTenant && hasPermission('catalog.manage');
  const key = masterDataKeys.units(principalId, tenantId);
  const [showInactive, setShowInactive] = useState(false);
  const [form, setForm] = useState<CreateUnitInput>(EMPTY);
  const [errors, setErrors] = useState<UnitValidationErrors>({});
  const [editing, setEditing] = useState<Unit | null>(null);
  const [editForm, setEditForm] = useState<Pick<CreateUnitInput, 'name' | 'symbol' | 'precision'>>({ name: '', symbol: '', precision: 0 });
  const [message, setMessage] = useState('');

  const unitsQuery = useQuery({ queryKey: key, queryFn: () => unitsApi.list(true), enabled: !!principalId && !!tenantId && canRead });
  const createUnit = useMutation({
    mutationFn: ({ input }: { principalId: string; tenantId: string; input: CreateUnitInput }) => unitsApi.create(input),
    onSuccess: async (_, variables) => {
      await queryClient.invalidateQueries({ queryKey: masterDataKeys.units(variables.principalId, variables.tenantId), exact: true });
      if (scope.current !== `${variables.principalId}:${variables.tenantId}`) return;
      setForm(EMPTY); setErrors({}); setMessage('Unit created.');
    },
  });
  const updateUnit = useMutation({
    mutationFn: ({ id, input }: { principalId: string; tenantId: string; id: string; input: Parameters<typeof unitsApi.update>[1] }) => unitsApi.update(id, input),
    onSuccess: async (_, variables) => {
      await queryClient.invalidateQueries({ queryKey: masterDataKeys.units(variables.principalId, variables.tenantId), exact: true });
      if (scope.current !== `${variables.principalId}:${variables.tenantId}`) return;
      setEditing(null); setMessage('Unit updated.');
    },
    onError: async (error, variables) => {
      if (isVersionConflict(error)) await queryClient.invalidateQueries({ queryKey: masterDataKeys.units(variables.principalId, variables.tenantId), exact: true });
    },
  });

  const resetCreate = createUnit.reset;
  const resetUpdate = updateUnit.reset;

  useLayoutEffect(() => {
    scope.current = scopeKey;
    setForm(EMPTY); setErrors({}); setEditing(null); setMessage('');
    resetCreate(); resetUpdate();
  }, [resetCreate, resetUpdate, scopeKey]);

  const submitCreate = (event: FormEvent) => {
    event.preventDefault();
    const validation = validateUnit(form);
    setErrors(validation);
    if (Object.keys(validation).length || !canManage) return;
    createUnit.mutate({ principalId, tenantId, input: normalizeUnit(form) });
  };
  const beginEdit = (unit: Unit) => {
    setEditing(unit); setEditForm({ name: unit.name, symbol: unit.symbol, precision: unit.precision }); setMessage(''); updateUnit.reset();
  };
  const submitEdit = (event: FormEvent) => {
    event.preventDefault();
    if (!editing || !canManage || !editForm.name.trim() || !editForm.symbol.trim() || editForm.precision < 0 || editForm.precision > 6) return;
    updateUnit.mutate({ principalId, tenantId, id: editing.id, input: { version: editing.version, name: editForm.name.trim(), symbol: editForm.symbol.trim(), precision: editForm.precision } });
  };
  const toggleActive = (unit: Unit) => {
    if (!canManage) return;
    updateUnit.mutate({ principalId, tenantId, id: unit.id, input: { version: unit.version, isActive: !unit.isActive } });
  };

  const units = (unitsQuery.data?.units ?? []).filter((unit) => showInactive || unit.isActive);
  const mutationError = createUnit.error || updateUnit.error;

  return <div className="space-y-8">
    <header><div className="mb-2 flex items-center gap-2 text-sm font-medium text-primary-400"><Database className="h-4 w-4" />Shared across all locations</div><h1 className="text-3xl font-bold tracking-tight text-white">Unit catalog</h1><p className="mt-2 max-w-2xl text-dark-400">Define exact base, purchase, and recipe units. Inventory quantities use checked micro-unit arithmetic rather than floating-point values.</p></header>
    <WorkspaceSettingsNav />
    {message && <Alert variant="success" className="flex items-center gap-2"><CheckCircle2 className="h-4 w-4" />{message}</Alert>}
    {mutationError && <Alert><span className="flex items-start gap-2"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />{isVersionConflict(mutationError) ? 'This unit changed elsewhere. The latest list has been loaded.' : getErrorMessage(mutationError)}</span></Alert>}
    {!profileLoading && !canManage && canRead && <Alert variant="info">You have read-only catalog access. A catalog manager can create or change units.</Alert>}

    {profileLoading ? <div className="flex justify-center py-16"><RefreshCw className="h-6 w-6 animate-spin text-primary-400" /></div>
      : profileError ? <Alert>{getErrorMessage(profileError)}</Alert>
      : missing || (!canRead && !canManage) ? <Alert>You do not have catalog access in this restaurant workspace.</Alert>
      : <div className={`grid gap-6 ${canManage ? 'lg:grid-cols-[minmax(0,1fr)_320px]' : ''}`}>
        <section className="overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/60">
          <div className="flex items-center justify-between border-b border-dark-800 px-5 py-4"><div><h2 className="font-semibold text-white">Units</h2><p className="mt-0.5 text-xs text-dark-500">Codes and dimensions are immutable after creation.</p></div><label className="flex items-center gap-2 text-xs text-dark-400"><input type="checkbox" checked={showInactive} onChange={(event) => setShowInactive(event.target.checked)} />Show inactive</label></div>
          {unitsQuery.isPending ? <div className="space-y-3 p-5">{[1, 2, 3].map((item) => <div key={item} className="h-16 animate-pulse rounded-xl bg-dark-800/70" />)}</div>
            : unitsQuery.isError ? <div className="p-8 text-center"><AlertCircle className="mx-auto h-8 w-8 text-red-400" /><p className="mt-3 text-sm text-dark-400">{getErrorMessage(unitsQuery.error)}</p><Button variant="secondary" className="mt-4" onClick={() => unitsQuery.refetch()}>Retry</Button></div>
            : units.length === 0 ? <div className="px-6 py-14 text-center"><Ruler className="mx-auto h-9 w-9 text-dark-600" /><h3 className="mt-3 font-medium text-white">No units yet</h3><p className="mt-1 text-sm text-dark-500">Create the first exact measurement unit for this tenant.</p></div>
            : <ul className="divide-y divide-dark-800">{units.map((unit) => <li key={unit.id} className="p-4 sm:p-5">{editing?.id === unit.id ? <form onSubmit={submitEdit} className="grid gap-3 sm:grid-cols-4"><Input label="Name" value={editForm.name} onChange={(event) => setEditForm((value) => ({ ...value, name: event.target.value }))} /><Input label="Symbol" value={editForm.symbol} onChange={(event) => setEditForm((value) => ({ ...value, symbol: event.target.value }))} /><Input label="Precision" type="number" min={0} max={6} value={editForm.precision} onChange={(event) => setEditForm((value) => ({ ...value, precision: Number(event.target.value) }))} /><div className="flex items-end gap-2"><Button type="submit" disabled={updateUnit.isPending} className="flex-1"><Save className="mr-1 inline h-4 w-4" />Save</Button><Button type="button" variant="secondary" onClick={() => setEditing(null)}>Cancel</Button></div></form> : <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><span className="font-semibold text-white">{unit.name}</span><code className="rounded bg-dark-800 px-1.5 py-0.5 text-xs text-primary-300">{unit.code}</code>{!unit.isActive && <span className="rounded-full bg-dark-800 px-2 py-0.5 text-xs text-dark-500">Inactive</span>}</div><p className="mt-1 text-sm text-dark-400">{unit.symbol} · {unit.dimension} · {unit.precision} decimal places</p></div>{canManage && <div className="flex gap-2"><Button variant="secondary" onClick={() => beginEdit(unit)}><Edit3 className="mr-1 inline h-4 w-4" />Edit</Button><Button variant="secondary" disabled={updateUnit.isPending} onClick={() => toggleActive(unit)}>{unit.isActive ? 'Deactivate' : 'Reactivate'}</Button></div>}</div>}</li>)}</ul>}
        </section>
        {canManage && <aside className="h-fit rounded-2xl border border-dark-800 bg-dark-900/60 p-5"><h2 className="flex items-center gap-2 font-semibold text-white"><Plus className="h-4 w-4 text-primary-400" />Add unit</h2><form onSubmit={submitCreate} className="mt-5 space-y-4"><Input label="Code" value={form.code} onChange={(event) => setForm((value) => ({ ...value, code: event.target.value.toLowerCase() }))} error={errors.code} placeholder="kg" maxLength={32} /><Input label="Name" value={form.name} onChange={(event) => setForm((value) => ({ ...value, name: event.target.value }))} error={errors.name} placeholder="Kilogram" /><Input label="Symbol" value={form.symbol} onChange={(event) => setForm((value) => ({ ...value, symbol: event.target.value }))} error={errors.symbol} placeholder="kg" /><Select label="Dimension" value={form.dimension} onChange={(event) => setForm((value) => ({ ...value, dimension: event.target.value as UnitDimension }))}>{DIMENSIONS.map((dimension) => <option key={dimension.value} value={dimension.value}>{dimension.label}</option>)}</Select><Input label="Decimal places" type="number" min={0} max={6} value={form.precision} onChange={(event) => setForm((value) => ({ ...value, precision: Number(event.target.value) }))} error={errors.precision} /><Button type="submit" disabled={createUnit.isPending} className="flex w-full items-center justify-center gap-2">{createUnit.isPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}Create unit</Button></form></aside>}
      </div>}
  </div>;
}
