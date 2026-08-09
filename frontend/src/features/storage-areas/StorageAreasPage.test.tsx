import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import StorageAreasPage from './StorageAreasPage';

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  permissions: [] as string[],
  profile: { status: 'active' } as { status: 'active' | 'inactive' } | null,
  missing: false,
  activeLocation: { id: 'location-a', name: 'Main', isActive: true },
}));

vi.mock('./api', () => ({ storageAreasApi: { list: mocks.list, create: vi.fn(), update: vi.fn() } }));
vi.mock('../../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'user-a' } }) }));
vi.mock('../../contexts/TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: 'tenant-a', tenantName: 'Bistro' }, role: 'user' }) }));
vi.mock('../../contexts/StaffProfileContext', () => ({ useStaffProfile: () => ({ profile: mocks.profile, loading: false, error: null, missing: mocks.missing, hasPermission: (permission: string) => mocks.permissions.includes(permission) }) }));
vi.mock('../../contexts/ActiveLocationContext', () => ({ useActiveLocation: () => ({ activeLocation: mocks.activeLocation, locations: [mocks.activeLocation], loading: false, error: null, setActiveLocation: vi.fn() }) }));

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/settings/storage-areas']}><StorageAreasPage /></MemoryRouter></QueryClientProvider>);
}

describe('StorageAreasPage capabilities', () => {
  beforeEach(() => {
    mocks.list.mockReset().mockResolvedValue({ storageAreas: [{ id: 'area-a', version: 1, name: 'Cold room', type: 'refrigerated', isActive: true }] });
    mocks.permissions = [];
    mocks.profile = { status: 'active' };
    mocks.missing = false;
    mocks.activeLocation = { id: 'location-a', name: 'Main', isActive: true };
  });

  it('shows a missing-profile state without fetching', () => {
    mocks.profile = null;
    mocks.missing = true;
    renderPage();
    expect(screen.getByRole('heading', { name: 'Staff profile required' })).toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('allows reading without rendering management controls', async () => {
    mocks.permissions = ['storage_areas.read'];
    renderPage();
    expect(await screen.findByText('Cold room')).toBeInTheDocument();
    expect(screen.getByText(/Read-only access/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Create storage area/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  });

  it('shows create and lifecycle controls with manage permission', async () => {
    mocks.permissions = ['storage_areas.read', 'storage_areas.manage'];
    renderPage();
    await waitFor(() => expect(mocks.list).toHaveBeenCalled());
    await screen.findByText('Cold room');
    expect(screen.getByRole('button', { name: 'Create storage area' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deactivate' })).toBeInTheDocument();
  });

  it('allows creating with manage permission when read permission is denied', () => {
    mocks.permissions = ['storage_areas.manage'];
    renderPage();
    expect(screen.getByText(/Create-only visibility/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Create storage area' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Areas at Main' })).not.toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
  });
});
