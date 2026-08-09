import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { teamKeys } from '../features/team/queries';
import { StaffProfileProvider, useStaffProfile } from './StaffProfileContext';

const mocks = vi.hoisted(() => ({ getSelf: vi.fn() }));
vi.mock('../features/staff-profiles/api', () => ({ staffProfilesApi: { getSelf: mocks.getSelf } }));

const inactiveResponse = {
  staffProfile: {
    id: '65a000000000000000000001', userId: '65a000000000000000000002', businessRole: 'viewer', allLocations: false, locationIds: [], permissionOverrides: [], status: 'inactive', version: 1,
    createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', effectivePermissions: [],
  },
};

function Probe() {
  const { profile, missing, error } = useStaffProfile();
  return <div>{profile?.status ?? (missing ? 'missing' : error ? 'error' : 'loading')}</div>;
}

function tree(client: QueryClient, principalId = 'user-a') {
  return <QueryClientProvider client={client}><StaffProfileProvider principalId={principalId} tenantId="tenant-a" enabled><Probe /></StaffProfileProvider></QueryClientProvider>;
}

describe('StaffProfileProvider backend responses', () => {
  beforeEach(() => mocks.getSelf.mockReset());

  it('exposes an inactive profile returned by the self endpoint', async () => {
    mocks.getSelf.mockResolvedValue(inactiveResponse);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(tree(client));
    expect(await screen.findByText('inactive')).toBeInTheDocument();
  });

  it('removes previous-principal Team caches on identity transition', async () => {
    mocks.getSelf.mockResolvedValue(inactiveResponse);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(teamKeys.members('user-a', 'tenant-a'), { members: [] });
    const view = render(tree(client));
    await screen.findByText('inactive');
    view.rerender(tree(client, 'user-b'));
    await waitFor(() => expect(client.getQueryData(teamKeys.members('user-a', 'tenant-a'))).toBeUndefined());
  });
});
