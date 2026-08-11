import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import UnitsPage from './UnitsPage';

const mocks = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), update: vi.fn(), canManage: true }));
vi.mock('../../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'user-a' } }) }));
vi.mock('../../contexts/TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: 'tenant-a' }, isRootTenant: false }) }));
vi.mock('../../contexts/StaffProfileContext', () => ({ useStaffProfile: () => ({ loading: false, error: null, missing: false, hasPermission: (permission: string) => permission === 'catalog.read' || mocks.canManage }) }));
vi.mock('./api', () => ({ unitsApi: { list: mocks.list, create: mocks.create, update: mocks.update } }));

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter><UnitsPage /></MemoryRouter></QueryClientProvider>);
}

describe('UnitsPage', () => {
  beforeEach(() => {
    mocks.canManage = true;
    mocks.list.mockReset().mockResolvedValue({ units: [] });
    mocks.create.mockReset().mockResolvedValue({ unit: { id: 'unit-a' } });
    mocks.update.mockReset();
  });

  it('creates a normalized tenant unit', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText('No units yet');
    await user.type(screen.getByLabelText('Code'), 'KG');
    await user.type(screen.getByLabelText('Name'), 'Kilogram');
    await user.type(screen.getByLabelText('Symbol'), 'kg');
    await user.click(screen.getByRole('button', { name: 'Create unit' }));
    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith({ code: 'kg', name: 'Kilogram', symbol: 'kg', dimension: 'mass', precision: 3 }));
  });

  it('keeps mutation controls hidden for catalog readers', async () => {
    mocks.canManage = false;
    mocks.list.mockResolvedValue({ units: [{ id: 'unit-a', code: 'kg', name: 'Kilogram', symbol: 'kg', dimension: 'mass', precision: 3, isActive: true, version: 1, createdAt: '', updatedAt: '' }] });
    renderPage();
    expect(await screen.findByText('Kilogram')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Create unit' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  });
});
