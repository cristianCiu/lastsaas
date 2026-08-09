import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import TenantBrandingPage from './TenantBrandingPage';

const { getBranding } = vi.hoisted(() => ({ getBranding: vi.fn() }));

vi.mock('./api', () => ({ tenantBrandingApi: { get: getBranding, update: vi.fn() } }));
vi.mock('../../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'user-a' } }) }));
vi.mock('../../contexts/TenantContext', () => ({
  useTenant: () => ({ activeTenant: { tenantId: 'tenant-a', tenantName: 'Bistro' }, role: 'admin' }),
}));

describe('TenantBrandingPage', () => {
  it('renders the scoped branding response and preview', async () => {
    getBranding.mockResolvedValueOnce({ branding: { primaryColor: '#112233', accentColor: '#abcdef', font: 'serif', version: 3 } });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/settings/branding']}><TenantBrandingPage /></MemoryRouter></QueryClientProvider>);
    expect(await screen.findByDisplayValue('#112233')).toBeInTheDocument();
    expect(screen.getByDisplayValue('#abcdef')).toBeInTheDocument();
    expect(screen.getByText('Published version 3. Empty fields inherit platform defaults.')).toBeInTheDocument();
    expect(screen.getByText('Bistro')).toBeInTheDocument();
  });
});
