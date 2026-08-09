import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import StorageAreasPage from './StorageAreasPage';

const { list } = vi.hoisted(() => ({ list: vi.fn() }));

vi.mock('./api', () => ({ storageAreasApi: { list, create: vi.fn(), update: vi.fn() } }));
vi.mock('../../contexts/TenantContext', () => ({
  useTenant: () => ({ activeTenant: { tenantId: 'tenant-a', tenantName: 'Bistro' }, role: 'admin' }),
}));
vi.mock('../../contexts/ActiveLocationContext', () => ({
  useActiveLocation: () => ({ activeLocation: null, locations: [], loading: false, error: null, setActiveLocation: vi.fn() }),
}));

describe('StorageAreasPage without an active location', () => {
  beforeEach(() => list.mockClear());

  it('explains the required selection without fetching storage areas', () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/settings/storage-areas']}><StorageAreasPage /></MemoryRouter></QueryClientProvider>);
    expect(screen.getByRole('heading', { name: 'Select an active location first' })).toBeInTheDocument();
    expect(list).not.toHaveBeenCalled();
  });
});
