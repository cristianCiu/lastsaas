import { useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { AlertCircle, CheckCircle2, Edit3, Link2, Plus, RefreshCw, Save } from 'lucide-react';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import WorkspaceSettingsNav from '../../components/WorkspaceSettingsNav';
import { useAuth } from '../../contexts/AuthContext';
import { useStaffProfile } from '../../contexts/StaffProfileContext';
import { useTenant } from '../../contexts/TenantContext';
import { getErrorMessage } from '../../utils/errors';
import { itemConversionsApi, itemsApi, unitsApi } from './api';
import { masterDataKeys } from './queries';
import type { CreateItemConversionInput, ItemConversion } from './types';
import { conversionPreview, normalizeConversion, validateConversion, type ConversionValidationErrors } from './conversions.validation';

const EMPTY: CreateItemConversionInput = { fromUnitId: '', numerator: '1', denominator: '1' };
const isConflict = (error: unknown) => axios.isAxiosError(error) && error.response?.data?.code === 'VERSION_CONFLICT';

export default function ConversionsPage() {
  const { user } = useAuth();
  const { activeTenant, isRootTenant } = useTenant();
  const { loading: profileLoading, error: profileError, missing, hasPermission } = useStaffProfile();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const scopeKey = `${principalId}:${tenantId}`;
  const scope = useRef(scopeKey);
  const client = useQueryClient();
  const canRead = !isRootTenant && hasPermission('catalog.read');
  const canManage = !isRootTenant && hasPermission('catalog.manage');
  const [itemId, setItemId] = useState('');
  const [form, setForm] = useState(EMPTY);
  const [errors, setErrors] = useState<ConversionValidationErrors>({});
  const [editing, setEditing] = useState<ItemConversion | null>(null);
  const [editForm, setEditForm] = useState(EMPTY);
  const [showInactive, setShowInactive] = useState(false);
  const [message, setMessage] = useState('');

  const items = useQuery({ queryKey: masterDataKeys.items(principalId, tenantId), queryFn: () => itemsApi.list(true), enabled: !!principalId && !!tenantId && canRead });
  const units = useQuery({ queryKey: masterDataKeys.units(principalId, tenantId), queryFn: () => unitsApi.list(true), enabled: !!principalId && !!tenantId && canRead });
  const conversionKey = masterDataKeys.itemConversions(principalId, tenantId, itemId);
  const conversions = useQuery({ queryKey: conversionKey, queryFn: () => itemConversionsApi.list(itemId), enabled: !!itemId && !!principalId && !!tenantId && canRead });
  const selectedItem = (items.data?.items ?? []).find((item) => item.id === itemId);
  const allUnits = units.data?.units ?? [];
  const baseUnit = allUnits.find((unit) => unit.id === selectedItem?.baseUnitId);
  const activeUnits = allUnits.filter((unit) => unit.isActive && unit.id !== selectedItem?.baseUnitId && unit.dimension === baseUnit?.dimension);
  const unitFor = (id: string) => allUnits.find((unit) => unit.id === id);
  const invalidate = () => client.invalidateQueries({ queryKey: conversionKey, exact: true });

  const create = useMutation({
    mutationFn: (input: CreateItemConversionInput) => itemConversionsApi.create(itemId, input),
    onSuccess: async () => { await invalidate(); setForm(EMPTY); setErrors({}); setMessage('Conversion created.'); },
  });
  const update = useMutation({
    mutationFn: ({ id, input }: { id: string; input: Parameters<typeof itemConversionsApi.update>[2] }) => itemConversionsApi.update(itemId, id, input),
    onSuccess: async () => { await invalidate(); setEditing(null); setMessage('Conversion updated.'); },
    onError: (error) => { if (isConflict(error)) void invalidate(); },
  });

  useLayoutEffect(() => {
    scope.current = scopeKey; setItemId(''); setForm(EMPTY); setErrors({}); setEditing(null); setMessage(''); create.reset(); update.reset();
  }, [scopeKey]);

  const submitCreate = (event: FormEvent) => {
    event.preventDefault(); const found = validateConversion(form); setErrors(found);
    if (Object.keys(found).length || !canManage || !itemId) return;
    create.mutate(normalizeConversion(form));
  };
  const beginEdit = (conversion: ItemConversion) => { setEditing(conversion); setEditForm({ fromUnitId: conversion.fromUnitId, numerator: conversion.numerator, denominator: conversion.denominator }); setMessage(''); update.reset(); };
  const submitEdit = (event: FormEvent) => {
    event.preventDefault(); if (!editing || !canManage) return;
    const found = validateConversion(editForm); if (Object.keys(found).length) return;
    update.mutate({ id: editing.id, input: { numerator: editForm.numerator.trim(), denominator: editForm.denominator.trim(), version: editing.version } });
  };
  const toggleActive = (conversion: ItemConversion) => { if (canManage) update.mutate({ id: conversion.id, input: { version: conversion.version, isActive: !conversion.isActive } }); };
  const list = (conversions.data?.conversions ?? []).filter((conversion) => showInactive || conversion.isActive);
  const mutationError = create.error || update.error;

  if (profileLoading) return <div className="flex justify-center py-16"><RefreshCw className="h-6 w-6 animate-spin text-primary-400" /></div>;
  return <div className="space-y-8">
    <header><div className="mb-2 flex items-center gap-2 text-sm font-medium text-primary-400"><Link2 className="h-4 w-4" />Shared across all locations</div><h1 className="text-3xl font-bold tracking-tight text-white">Item conversions</h1><p className="mt-2 max-w-2xl text-dark-400">Define how purchasing units convert to an item’s base unit.</p></header>
    <WorkspaceSettingsNav />
    {message && <Alert variant="success" className="flex items-center gap-2"><CheckCircle2 className="h-4 w-4" />{message}</Alert>}
    {mutationError && <Alert>{isConflict(mutationError) ? 'This conversion changed elsewhere. The latest list has been loaded.' : getErrorMessage(mutationError)}</Alert>}
    {!canManage && canRead && <Alert variant="info">You have read-only catalog access. A catalog manager can create or change conversions.</Alert>}
    {profileError ? <Alert>{getErrorMessage(profileError)}</Alert> : missing || (!canRead && !canManage) ? <Alert>You do not have catalog access in this restaurant workspace.</Alert> : <>
      <div className="rounded-2xl border border-dark-800 bg-dark-900/60 p-5"><Select label="Item" value={itemId} onChange={(e) => { setItemId(e.target.value); setEditing(null); setMessage(''); }}><option value="">Choose an active item</option>{(items.data?.items ?? []).filter((item) => item.isActive).map((item) => <option key={item.id} value={item.id}>{item.name} ({item.sku})</option>)}</Select>{selectedItem && <p className="mt-3 text-sm text-dark-400">Base unit: <span className="text-dark-200">{baseUnit?.name ?? 'Unavailable'}{baseUnit?.isActive === false ? ' (Inactive)' : ''}</span></p>}</div>
      {itemId && <div className={`grid gap-6 ${canManage ? 'lg:grid-cols-[minmax(0,1fr)_340px]' : ''}`}><section className="overflow-hidden rounded-2xl border border-dark-800 bg-dark-900/60"><div className="flex items-center justify-between border-b border-dark-800 px-5 py-4"><div><h2 className="font-semibold text-white">Conversions</h2><p className="mt-0.5 text-xs text-dark-500">Factors are exact whole numbers.</p></div><label className="flex items-center gap-2 text-xs text-dark-400"><input type="checkbox" checked={showInactive} onChange={(e) => setShowInactive(e.target.checked)} />Show inactive</label></div>{conversions.isPending ? <div className="p-5"><div className="h-16 animate-pulse rounded-xl bg-dark-800/70" /></div> : conversions.isError ? <div className="p-8 text-center"><AlertCircle className="mx-auto h-8 w-8 text-red-400" /><p className="mt-3 text-sm text-dark-400">{getErrorMessage(conversions.error)}</p><Button variant="secondary" className="mt-4" onClick={() => conversions.refetch()}>Retry</Button></div> : list.length === 0 ? <div className="px-6 py-14 text-center"><Link2 className="mx-auto h-9 w-9 text-dark-600" /><h3 className="mt-3 font-medium text-white">No conversions yet</h3><p className="mt-1 text-sm text-dark-500">Add the first source-unit conversion for this item.</p></div> : <ul className="divide-y divide-dark-800">{list.map((conversion) => { const source = unitFor(conversion.fromUnitId); return <li key={conversion.id} className="p-4 sm:p-5">{editing?.id === conversion.id ? <form onSubmit={submitEdit} className="space-y-4"><p className="text-sm text-dark-400">Source unit: <span className="text-dark-200">{source?.name ?? 'Unavailable'}{source?.isActive === false ? ' (Inactive)' : ''}</span></p><div className="grid grid-cols-2 gap-3"><Input label="Numerator" value={editForm.numerator} onChange={(e) => setEditForm((v) => ({ ...v, numerator: e.target.value }))} inputMode="numeric" /><Input label="Denominator" value={editForm.denominator} onChange={(e) => setEditForm((v) => ({ ...v, denominator: e.target.value }))} inputMode="numeric" /></div><p className="text-xs text-dark-500">{conversionPreview(editForm.numerator, editForm.denominator, source?.name ?? 'source unit', baseUnit?.name ?? 'base unit')}</p><div className="flex gap-2"><Button type="submit" disabled={update.isPending}><Save className="mr-1 inline h-4 w-4" />Save</Button><Button type="button" variant="secondary" onClick={() => setEditing(null)}>Cancel</Button></div></form> : <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><p className="font-medium text-white">{conversionPreview(conversion.numerator, conversion.denominator, source?.name ?? 'Unavailable unit', baseUnit?.name ?? 'base unit')}</p>{!conversion.isActive && <span className="mt-1 inline-block rounded-full bg-dark-800 px-2 py-0.5 text-xs text-dark-500">Inactive</span>}</div>{canManage && <div className="flex gap-2"><Button variant="secondary" onClick={() => beginEdit(conversion)}><Edit3 className="mr-1 inline h-4 w-4" />Edit</Button><Button variant="secondary" disabled={update.isPending} onClick={() => toggleActive(conversion)}>{conversion.isActive ? 'Deactivate' : 'Reactivate'}</Button></div>}</div>}</li>; })}</ul>}</section>{canManage && <aside className="h-fit rounded-2xl border border-dark-800 bg-dark-900/60 p-5"><h2 className="flex items-center gap-2 font-semibold text-white"><Plus className="h-4 w-4 text-primary-400" />Add conversion</h2><form onSubmit={submitCreate} className="mt-5 space-y-4"><Select label="Source unit" value={form.fromUnitId} onChange={(e) => setForm((v) => ({ ...v, sourceUnitId: e.target.value }))} error={errors.fromUnitId}><option value="">Choose source unit</option>{activeUnits.map((unit) => <option key={unit.id} value={unit.id}>{unit.name} ({unit.symbol})</option>)}</Select><div className="grid grid-cols-2 gap-3"><Input label="Numerator" value={form.numerator} onChange={(e) => setForm((v) => ({ ...v, numerator: e.target.value }))} error={errors.numerator} inputMode="numeric" /><Input label="Denominator" value={form.denominator} onChange={(e) => setForm((v) => ({ ...v, denominator: e.target.value }))} error={errors.denominator} inputMode="numeric" /></div><p className="text-xs text-dark-500">{conversionPreview(form.numerator || '0', form.denominator || '0', unitFor(form.fromUnitId)?.name ?? 'source unit', baseUnit?.name ?? 'base unit')}</p><Button type="submit" disabled={create.isPending} className="flex w-full items-center justify-center gap-2">{create.isPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}Create conversion</Button></form></aside>}</div>}
    </>}
  </div>;
}
