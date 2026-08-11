import { useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { Crown, MapPin, Pencil, ShieldCheck, Trash2, User, UserPlus, Users } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { plansApi, tenantApi } from '../../api/client';
import ConfirmModal from '../../components/ConfirmModal';
import LoadingSpinner from '../../components/LoadingSpinner';
import Alert from '../../components/ui/Alert';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import { useActiveLocation } from '../../contexts/ActiveLocationContext';
import { useAuth } from '../../contexts/AuthContext';
import { useTenant } from '../../contexts/TenantContext';
import { locationKeys } from '../../features/locations/queries';
import { staffProfilesApi } from '../../features/staff-profiles/api';
import { staffProfileKeys } from '../../features/staff-profiles/queries';
import { BUSINESS_PERMISSIONS, BUSINESS_ROLES, type BusinessPermission, type BusinessRole, type PermissionOverride, type StaffProfile, type StaffProfileStatus, type UpdateStaffProfileInput } from '../../features/staff-profiles/types';
import { storageAreaKeys } from '../../features/storage-areas/queries';
import { teamKeys } from '../../features/team/queries';
import type { TenantMember } from '../../types';
import { getErrorMessage } from '../../utils/errors';

const roleIcons = { owner: Crown, admin: ShieldCheck, user: User };
const BUSINESS_ROLE_LABELS: Record<BusinessRole, string> = {
  company_owner: 'Company owner', operations_manager: 'Operations manager', head_chef: 'Head chef', purchasing: 'Purchasing', stock_service: 'Stock service', controller: 'Controller', viewer: 'Viewer',
};

function renderTemplate(template: string, vars: Record<string, string | number>): string {
  let result = template;
  for (const [key, value] of Object.entries(vars)) result = result.replace(new RegExp(`\\{\\{\\.${key}\\}\\}`, 'g'), String(value));
  return result.replace(/\{\{if ne \.(\w+) (\d+)\}\}(.*?)\{\{end\}\}/g, (_match, varName, compare, content) => String(vars[varName]) !== compare ? content : '');
}

function overrideValue(overrides: PermissionOverride[], permission: BusinessPermission): 'inherit' | 'allow' | 'deny' {
  const override = overrides.find((item) => item.permission === permission);
  return override ? override.allowed ? 'allow' : 'deny' : 'inherit';
}

function ProfileEditor({ profile, locations, saving, error, onCancel, onSave }: {
  profile: StaffProfile;
  locations: ReturnType<typeof useActiveLocation>['locations'];
  saving: boolean;
  error: string;
  onCancel: () => void;
  onSave: (input: UpdateStaffProfileInput) => void;
}) {
  const [businessRole, setBusinessRole] = useState(profile.businessRole);
  const [allLocations, setAllLocations] = useState(profile.allLocations);
  const [locationIds, setLocationIds] = useState(profile.locationIds);
  const [status, setStatus] = useState<StaffProfileStatus>(profile.status);
  const [overrides, setOverrides] = useState(profile.permissionOverrides);
  const unavailableLocationIds = locationIds.filter((id) => !locations.some((location) => location.id === id));

  const setOverride = (permission: BusinessPermission, value: 'inherit' | 'allow' | 'deny') => {
    setOverrides((current) => value === 'inherit'
      ? current.filter((item) => item.permission !== permission)
      : [...current.filter((item) => item.permission !== permission), { permission, allowed: value === 'allow' }]);
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSave({ version: profile.version, businessRole, allLocations, locationIds: allLocations ? [] : locationIds, permissionOverrides: overrides, status });
  };

  return <form className="mt-5 space-y-5 border-t border-dark-800 pt-5" onSubmit={submit}>
    {error && <Alert>{error}</Alert>}
    <div className="grid gap-4 sm:grid-cols-2">
      <Select label="Business role" value={businessRole} disabled={saving} onChange={(event) => setBusinessRole(event.target.value as BusinessRole)}>
        {BUSINESS_ROLES.map((role) => <option key={role} value={role}>{BUSINESS_ROLE_LABELS[role]}</option>)}
      </Select>
      <Select label="Profile status" value={status} disabled={saving} onChange={(event) => setStatus(event.target.value as StaffProfileStatus)}>
        <option value="active">Active</option><option value="inactive">Inactive</option>
      </Select>
    </div>
    <fieldset disabled={saving}>
      <legend className="mb-2 text-sm font-medium text-dark-300">Location access</legend>
      <label className="flex items-center gap-2 rounded-lg border border-dark-700 bg-dark-800/60 p-3 text-sm text-white"><input type="checkbox" checked={allLocations} onChange={(event) => setAllLocations(event.target.checked)} />All locations</label>
      {!allLocations && <><div className="mt-2 grid gap-2 sm:grid-cols-2">{locations.map((location) => <label key={location.id} className="flex items-center gap-2 rounded-lg border border-dark-800 p-3 text-sm text-dark-300"><input type="checkbox" checked={locationIds.includes(location.id)} onChange={(event) => setLocationIds((current) => event.target.checked ? [...new Set([...current, location.id])] : current.filter((id) => id !== location.id))} /><span>{location.name}{!location.isActive && <span className="ml-1 text-dark-500">(inactive)</span>}</span></label>)}{unavailableLocationIds.map((id) => <label key={id} className="flex items-center gap-2 rounded-lg border border-amber-500/20 bg-amber-500/5 p-3 text-sm text-amber-300"><input type="checkbox" checked onChange={() => setLocationIds((current) => current.filter((locationId) => locationId !== id))} />Unavailable location ({id})</label>)}</div><p className="mt-2 text-xs text-dark-500">Leaving all locations unchecked removes location access.</p></>}
    </fieldset>
    <fieldset className="grid gap-3 sm:grid-cols-2" disabled={saving}>
      <legend className="mb-2 text-sm font-medium text-dark-300 sm:col-span-2">Storage permissions</legend>
      {BUSINESS_PERMISSIONS.map((permission) => <Select key={permission} label={{ 'storage_areas.read': 'View storage areas', 'storage_areas.manage': 'Manage storage areas', 'catalog.read': 'View catalog', 'catalog.manage': 'Manage catalog' }[permission]} value={overrideValue(overrides, permission)} onChange={(event) => setOverride(permission, event.target.value as 'inherit' | 'allow' | 'deny')}><option value="inherit">Default / inherit</option><option value="allow">Allow</option><option value="deny">Deny</option></Select>)}
    </fieldset>
    <div className="flex justify-end gap-2"><Button type="button" variant="ghost" onClick={onCancel} disabled={saving}>Cancel</Button><Button type="submit" disabled={saving}>{saving ? 'Saving...' : 'Save access profile'}</Button></div>
  </form>;
}

export default function TeamPage() {
  const { user } = useAuth();
  const { activeTenant, role: myRole } = useTenant();
  const { locations } = useActiveLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const principalId = user?.id ?? '';
  const tenantId = activeTenant?.tenantId ?? '';
  const scopeRef = useRef({ principalId, tenantId });
  const isCurrentScope = (scope: { principalId: string; tenantId: string }) => scopeRef.current.principalId === scope.principalId && scopeRef.current.tenantId === scope.tenantId;
  const canManageCore = myRole === 'owner' || myRole === 'admin';
  const isOwner = myRole === 'owner';
  const membersQuery = useQuery({ queryKey: teamKeys.members(principalId, tenantId), queryFn: () => tenantApi.listMembers(), enabled: !!principalId && !!tenantId });
  const profilesQuery = useQuery({ queryKey: staffProfileKeys.list(principalId, tenantId), queryFn: () => staffProfilesApi.list(), enabled: canManageCore && !!principalId && !!tenantId });
  const plansQuery = useQuery({ queryKey: teamKeys.plans(principalId, tenantId), queryFn: () => plansApi.list(), enabled: !!principalId && !!tenantId });
  const members = membersQuery.data?.members ?? [];
  const profiles = profilesQuery.data?.staffProfiles ?? [];
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState('user');
  const [showInvite, setShowInvite] = useState(false);
  const [message, setMessage] = useState<{ type: 'error' | 'success'; text: string } | null>(null);
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [removeMember, setRemoveMember] = useState<TenantMember | null>(null);
  const [editingUserId, setEditingUserId] = useState<string | null>(null);
  const [profileError, setProfileError] = useState('');

  const inviteMutation = useMutation({ mutationFn: ({ email, role }: { email: string; role: string; principalId: string; tenantId: string }) => tenantApi.inviteMember(email, role) });
  const removeMutation = useMutation({
    mutationFn: ({ member }: { member: TenantMember; principalId: string; tenantId: string }) => tenantApi.removeMember(member.userId),
    onSuccess: async (_, variables) => { await queryClient.invalidateQueries({ queryKey: teamKeys.members(variables.principalId, variables.tenantId), exact: true }); if (!isCurrentScope(variables)) return; toast.success(`${variables.member.displayName} removed from team`); setRemoveMember(null); },
    onError: (error, variables) => { if (isCurrentScope(variables)) toast.error(getErrorMessage(error)); },
  });
  const roleMutation = useMutation({
    mutationFn: ({ member, role }: { member: TenantMember; role: string; principalId: string; tenantId: string }) => tenantApi.changeRole(member.userId, role),
    onSuccess: async (_, variables) => { await queryClient.invalidateQueries({ queryKey: teamKeys.members(variables.principalId, variables.tenantId), exact: true }); if (isCurrentScope(variables)) toast.success(`${variables.member.displayName}'s workspace role changed`); },
    onError: (error, variables) => { if (isCurrentScope(variables)) toast.error(getErrorMessage(error)); },
  });
  const profileMutation = useMutation({
    mutationFn: ({ userId, input }: { userId: string; input: UpdateStaffProfileInput; principalId: string; tenantId: string }) => staffProfilesApi.update(userId, input),
    onSuccess: async (_, variables) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: staffProfileKeys.list(variables.principalId, variables.tenantId), exact: true }),
        variables.userId === variables.principalId ? queryClient.invalidateQueries({ queryKey: staffProfileKeys.self(variables.principalId, variables.tenantId), exact: true }) : Promise.resolve(),
        queryClient.invalidateQueries({ queryKey: locationKeys.list(variables.principalId, variables.tenantId), exact: true }),
        queryClient.invalidateQueries({ queryKey: [...storageAreaKeys.all, 'list', variables.principalId, variables.tenantId] }),
      ]);
      if (!isCurrentScope(variables)) return;
      setEditingUserId(null); setProfileError(''); toast.success('Staff access profile updated');
    },
    onError: async (error, variables) => {
      if (axios.isAxiosError(error) && error.response?.data?.code === 'VERSION_CONFLICT') {
        await queryClient.invalidateQueries({ queryKey: staffProfileKeys.list(variables.principalId, variables.tenantId), exact: true });
        if (!isCurrentScope(variables)) return;
        setEditingUserId(null);
        toast.error('This profile changed elsewhere. The latest version has been loaded.');
      } else if (isCurrentScope(variables)) setProfileError(getErrorMessage(error));
    },
  });

  const resetInvite = inviteMutation.reset;
  const resetRemove = removeMutation.reset;
  const resetRole = roleMutation.reset;
  const resetProfile = profileMutation.reset;
  useLayoutEffect(() => {
    scopeRef.current = { principalId, tenantId };
    setShowInvite(false);
    setShowUpgradeModal(false);
    setInviteEmail('');
    setInviteRole('user');
    setEditingUserId(null);
    setRemoveMember(null);
    setMessage(null);
    setProfileError('');
    resetInvite();
    resetRemove();
    resetRole();
    resetProfile();
  }, [principalId, tenantId, resetInvite, resetRemove, resetRole, resetProfile]);

  const submitInvite = async (event: FormEvent) => {
    event.preventDefault(); setMessage(null);
    if (!isOwner && inviteRole === 'admin') return;
    const scope = { principalId, tenantId };
    try { await inviteMutation.mutateAsync({ email: inviteEmail, role: inviteRole, ...scope }); if (!isCurrentScope(scope)) return; setMessage({ type: 'success', text: `Invitation sent to ${inviteEmail}` }); setInviteEmail(''); setShowInvite(false); }
    catch (error) { if (!isCurrentScope(scope)) return; const data = (error as { response?: { data?: { error?: string; code?: string } } })?.response?.data; if (data?.code === 'USER_LIMIT_REACHED') setShowUpgradeModal(true); else setMessage({ type: 'error', text: data?.error || 'Failed to send invitation' }); }
  };

  if (membersQuery.isPending) return <LoadingSpinner size="lg" className="py-20" />;
  if (membersQuery.isError) return <Alert>{getErrorMessage(membersQuery.error)}</Alert>;

  const planData = plansQuery.data;
  const sortedPlans = [...(planData?.plans ?? [])].sort((a, b) => a.monthlyPriceCents - b.monthlyPriceCents);
  const currentPlanIndex = sortedPlans.findIndex((plan) => plan.id === planData?.currentPlanId);
  const recommendedPlan = sortedPlans.slice(currentPlanIndex + 1).find((plan) => plan.userLimit === 0 || plan.userLimit > members.length);
  const upgradeVars = { UserLimit: planData?.currentPlanUserLimit ?? 0, PlanName: sortedPlans.find((plan) => plan.id === planData?.currentPlanId)?.name || '' };
  const rolePending = roleMutation.isPending && roleMutation.variables?.principalId === principalId && roleMutation.variables.tenantId === tenantId;
  return <div className="space-y-6">
    <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"><div><div className="mb-2 flex items-center gap-2 text-sm font-medium text-primary-400"><Users className="h-4 w-4" />Workspace administration</div><h1 className="text-3xl font-bold tracking-tight text-white">Team & access</h1><p className="mt-2 text-dark-400">{members.length} members. Workspace authority and operational access are managed separately.</p></div>{canManageCore && <Button onClick={() => setShowInvite((open) => !open)} className="inline-flex items-center justify-center gap-2 self-start sm:self-auto"><UserPlus className="h-4 w-4" />Invite member</Button>}</header>
    {message && <Alert variant={message.type === 'success' ? 'success' : 'error'}>{message.text}</Alert>}
    {showInvite && <form onSubmit={submitInvite} className="grid gap-4 rounded-2xl border border-dark-800 bg-dark-900/60 p-5 sm:grid-cols-[minmax(0,1fr)_10rem_auto] sm:items-end"><Input type="email" required label="Email address" value={inviteEmail} onChange={(event) => setInviteEmail(event.target.value)} placeholder="teammate@example.com" /><Select label="Workspace role" value={inviteRole} onChange={(event) => setInviteRole(event.target.value)}><option value="user">User</option>{isOwner && <option value="admin">Admin</option>}</Select><Button type="submit" disabled={inviteMutation.isPending}>{inviteMutation.isPending ? 'Sending...' : 'Send invite'}</Button></form>}
    {profilesQuery.isError && <Alert>Staff profiles could not be loaded: {getErrorMessage(profilesQuery.error)}</Alert>}
    <div className="grid gap-4 xl:grid-cols-2">{members.map((member) => {
      const RoleIcon = roleIcons[member.role];
      const isMe = member.userId === principalId;
      const profile = profiles.find((item) => item.userId === member.userId);
      const canEditProfile = !!profile && ((isOwner && member.role !== 'owner') || (myRole === 'admin' && member.role === 'user'));
      const canRemove = !isMe && member.role !== 'owner' && (isOwner || (myRole === 'admin' && member.role === 'user'));
      return <article key={member.userId} className="min-w-0 rounded-2xl border border-dark-800 bg-dark-900/55 p-5">
        <div className="flex min-w-0 items-start justify-between gap-3"><div className="min-w-0"><h2 className="truncate font-semibold text-white">{member.displayName}{isMe && <span className="ml-2 text-xs font-normal text-dark-500">you</span>}</h2><p className="truncate text-sm text-dark-500">{member.email}</p></div>{canRemove && <button onClick={() => setRemoveMember(member)} className="rounded-lg p-2 text-dark-500 hover:bg-red-500/10 hover:text-red-400" aria-label={`Remove ${member.displayName}`}><Trash2 className="h-4 w-4" /></button>}</div>
        <div className="mt-5 grid gap-4 sm:grid-cols-2"><div className="rounded-xl border border-dark-800 bg-dark-950/40 p-4"><p className="text-[11px] font-medium uppercase tracking-wider text-dark-500">Workspace role</p><div className="mt-2 flex items-center gap-2"><RoleIcon className="h-4 w-4 text-primary-400" />{isOwner && !isMe && member.role !== 'owner' ? <Select aria-label={`Workspace role for ${member.displayName}`} value={member.role} disabled={rolePending} onChange={(event) => roleMutation.mutate({ member, role: event.target.value, principalId, tenantId })}><option value="user">User</option><option value="admin">Admin</option></Select> : <span className="capitalize text-white">{member.role}</span>}</div><p className="mt-2 text-xs text-dark-500">Core membership authority</p></div>
          <div className="rounded-xl border border-dark-800 bg-dark-950/40 p-4"><p className="text-[11px] font-medium uppercase tracking-wider text-dark-500">Staff profile</p>{profile ? <><p className="mt-2 font-medium text-white">{BUSINESS_ROLE_LABELS[profile.businessRole]}</p><p className="mt-1 flex items-center gap-1 text-xs text-dark-500"><MapPin className="h-3 w-3" />{profile.allLocations ? 'All locations' : `${profile.locationIds.length} assigned`}{profile.status === 'inactive' && ' · Inactive'}</p></> : <p className="mt-2 text-sm text-dark-500">Not configured</p>}</div></div>
        <div className="mt-4 flex items-center justify-between"><p className="text-xs text-dark-600">Joined {new Date(member.joinedAt).toLocaleDateString()}</p>{canEditProfile && <Button size="sm" variant="secondary" onClick={() => { setEditingUserId(member.userId); setProfileError(''); }} className="inline-flex items-center gap-1.5"><Pencil className="h-3.5 w-3.5" />Edit staff access</Button>}</div>
        {editingUserId === member.userId && profile && <ProfileEditor key={`${profile.userId}:${profile.version}`} profile={profile} locations={locations} saving={profileMutation.isPending} error={profileError} onCancel={() => setEditingUserId(null)} onSave={(input) => profileMutation.mutate({ userId: member.userId, input, principalId, tenantId })} />}
      </article>;
    })}</div>
    <ConfirmModal open={showUpgradeModal && !!planData} onClose={() => setShowUpgradeModal(false)} onConfirm={() => { setShowUpgradeModal(false); navigate(recommendedPlan ? `/plan?upgrade=${recommendedPlan.id}` : '/plan'); }} title={renderTemplate(planData?.upgradePromptTitle ?? 'Upgrade plan', upgradeVars)} message={renderTemplate(planData?.upgradePromptBody ?? 'Upgrade your plan to add more members.', upgradeVars)} confirmLabel="Upgrade plan" />
    <ConfirmModal open={!!removeMember} onClose={() => setRemoveMember(null)} onConfirm={() => removeMember && removeMutation.mutate({ member: removeMember, principalId, tenantId })} title="Remove team member" message={`Are you sure you want to remove ${removeMember?.displayName} from the team?`} confirmLabel="Remove" confirmVariant="danger" loading={removeMutation.isPending} />
  </div>;
}
