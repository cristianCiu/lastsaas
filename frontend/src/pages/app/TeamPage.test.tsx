import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { staffProfileKeys } from '../../features/staff-profiles/queries';
import { teamKeys } from '../../features/team/queries';
import TeamPage from './TeamPage';

const mocks = vi.hoisted(() => ({
  role: 'admin' as 'owner' | 'admin' | 'user',
  tenantId: 'tenant-a',
  listMembers: vi.fn(),
  listProfiles: vi.fn(),
  listPlans: vi.fn(),
  inviteMember: vi.fn(),
  changeRole: vi.fn(),
}));
const membersResponse = { members: [
  { userId: 'admin-a', email: 'me@example.com', displayName: 'Current Admin', role: 'admin' as const, joinedAt: '2026-01-01T00:00:00Z' },
  { userId: 'admin-b', email: 'admin@example.com', displayName: 'Other Admin', role: 'admin' as const, joinedAt: '2026-01-01T00:00:00Z' },
  { userId: 'user-a', email: 'user@example.com', displayName: 'Core User', role: 'user' as const, joinedAt: '2026-01-01T00:00:00Z' },
] };
const profilesResponse = { staffProfiles: [
  { id: 'profile-user', userId: 'user-a', businessRole: 'viewer' as const, allLocations: false, locationIds: ['location-old'], permissionOverrides: [], status: 'active' as const, version: 1, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', effectivePermissions: [] },
  { id: 'profile-admin', userId: 'admin-b', businessRole: 'operations_manager' as const, allLocations: true, locationIds: [], permissionOverrides: [], status: 'active' as const, version: 1, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', effectivePermissions: [] },
] };
const plansResponse = { plans: [], currentPlanId: '', currentPlanUserLimit: 0, upgradePromptTitle: '', upgradePromptBody: '' };

vi.mock('../../api/client', () => ({
  tenantApi: { listMembers: mocks.listMembers, inviteMember: mocks.inviteMember, removeMember: vi.fn(), changeRole: mocks.changeRole },
  plansApi: { list: mocks.listPlans },
}));
vi.mock('../../features/staff-profiles/api', () => ({ staffProfilesApi: { list: mocks.listProfiles, update: vi.fn() } }));
vi.mock('../../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'admin-a' } }) }));
vi.mock('../../contexts/TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: mocks.tenantId }, role: mocks.role }) }));
vi.mock('../../contexts/ActiveLocationContext', () => ({ useActiveLocation: () => ({ locations: [{ id: 'location-old', name: 'Old kitchen', isActive: false }], activeLocation: null }) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function renderPage(client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })) {
  return render(<TestProviders client={client} />);
}

function TestProviders({ client }: { client: QueryClient }) {
  return <QueryClientProvider client={client}><MemoryRouter><TeamPage /></MemoryRouter></QueryClientProvider>;
}

describe('TeamPage workspace restrictions', () => {
  beforeEach(() => {
    mocks.role = 'admin';
    mocks.tenantId = 'tenant-a';
    mocks.listMembers.mockReset().mockResolvedValue(membersResponse);
    mocks.listProfiles.mockReset().mockResolvedValue(profilesResponse);
    mocks.listPlans.mockReset().mockResolvedValue(plansResponse);
    mocks.inviteMember.mockReset().mockResolvedValue({});
    mocks.changeRole.mockReset().mockResolvedValue({});
  });

  it('distinguishes workspace and staff roles while preventing admin-on-admin actions', () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } });
    client.setQueryData(teamKeys.members('admin-a', 'tenant-a'), membersResponse);
    client.setQueryData(staffProfileKeys.list('admin-a', 'tenant-a'), profilesResponse);
    client.setQueryData(teamKeys.plans('admin-a', 'tenant-a'), plansResponse);
    renderPage(client);
    const adminCard = screen.getByText('Other Admin').closest('article');
    const userCard = screen.getByText('Core User').closest('article');
    expect(adminCard).not.toBeNull();
    expect(userCard).not.toBeNull();
    const adminView = within(adminCard!);
    const userView = within(userCard!);
    expect(adminView.getByText('Workspace role')).toBeInTheDocument();
    expect(adminView.getByText('Staff profile')).toBeInTheDocument();
    expect(adminView.queryByLabelText('Remove Other Admin')).not.toBeInTheDocument();
    expect(adminView.queryByText('Edit staff access')).not.toBeInTheDocument();
    expect(userView.getByLabelText('Remove Core User')).toBeInTheDocument();
    const editButton = userView.getByText('Edit staff access').closest('button');
    expect(editButton).not.toBeNull();

    fireEvent.click(screen.getByText('Invite member').closest('button')!);
    const roleSelect = screen.getByLabelText('Workspace role');
    expect(roleSelect).toHaveTextContent('User');
    expect(roleSelect).not.toHaveTextContent('Admin');
    fireEvent.click(editButton!);
    expect(userView.getByLabelText(/Old kitchen/)).toBeChecked();
    expect(userView.getByText('(inactive)')).toBeInTheDocument();
  });

  it('suppresses an old tenant invite response after the tenant changes', async () => {
    let resolveInvite!: (value: object) => void;
    mocks.inviteMember.mockReturnValue(new Promise((resolve) => { resolveInvite = resolve; }));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    const view = renderPage(client);
    const user = userEvent.setup();
    await screen.findByText('Current Admin');
    await user.click(screen.getByRole('button', { name: 'Invite member' }));
    await user.type(screen.getByLabelText('Email address'), 'old@example.com');
    await user.click(screen.getByRole('button', { name: 'Send invite' }));

    mocks.tenantId = 'tenant-b';
    view.rerender(<TestProviders client={client} />);
    await act(async () => resolveInvite({}));
    expect(screen.queryByText(/Invitation sent to old@example.com/)).not.toBeInTheDocument();
  });

  it('allows owners to edit non-owner profiles and disables role controls while saving', async () => {
    mocks.role = 'owner';
    let resolveRole!: (value: object) => void;
    mocks.changeRole.mockReturnValue(new Promise((resolve) => { resolveRole = resolve; }));
    const user = userEvent.setup();
    renderPage();
    await screen.findByText('Other Admin');
    expect(screen.getAllByRole('button', { name: /Edit staff access/ })).toHaveLength(2);

    const roleSelect = screen.getByLabelText('Workspace role for Other Admin');
    await user.selectOptions(roleSelect, 'user');
    expect(roleSelect).toBeDisabled();
    await act(async () => resolveRole({}));
  });
});
