import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import TenantBrandingPage from './TenantBrandingPage';

const mocks = vi.hoisted(() => ({ getBranding: vi.fn(), uploadAsset: vi.fn(), deleteAsset: vi.fn() }));

vi.mock('./api', () => ({ tenantBrandingApi: { get: mocks.getBranding, update: vi.fn(), uploadAsset: mocks.uploadAsset, deleteAsset: mocks.deleteAsset } }));
vi.mock('../../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'user-a' } }) }));
vi.mock('../../contexts/TenantContext', () => ({
  useTenant: () => ({ activeTenant: { tenantId: 'tenant-a', tenantName: 'Bistro' }, role: 'admin', isRootTenant: false }),
}));
vi.mock('../../contexts/TenantBrandingContext', () => ({ useTenantBranding: () => ({ assets: [], assetsLoading: false, assetsError: null, primaryLogoUrl: null, compactLogoUrl: null }) }));

describe('TenantBrandingPage', () => {
  it('renders the scoped branding response and preview', async () => {
    mocks.getBranding.mockResolvedValueOnce({ branding: { primaryColor: '#112233', accentColor: '#abcdef', font: 'serif', version: 3 } });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/settings/branding']}><TenantBrandingPage /></MemoryRouter></QueryClientProvider>);
    expect(await screen.findByDisplayValue('#112233')).toBeInTheDocument();
    expect(screen.getByDisplayValue('#abcdef')).toBeInTheDocument();
    expect(screen.getByText('Published version 3. Empty fields inherit platform defaults.')).toBeInTheDocument();
    expect(screen.getByText('Bistro')).toBeInTheDocument();
  });

  it('uploads the primary logo with the safe file contract', async () => {
    mocks.getBranding.mockResolvedValueOnce({ branding: { primaryColor: '', accentColor: '', font: '', version: 0 } });
    mocks.uploadAsset.mockResolvedValueOnce({ asset: { id: 'asset-a', kind: 'primary', contentType: 'image/png', size: 4, width: 64, height: 64, version: 1, createdAt: '', updatedAt: '' } });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const user = userEvent.setup();
    render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/settings/branding']}><TenantBrandingPage /></MemoryRouter></QueryClientProvider>);
    await screen.findByText('Primary logo');
    const file = new File(['logo'], 'logo.png', { type: 'image/png' });
    await user.upload(screen.getByLabelText('Primary logo'), file);
    expect(mocks.uploadAsset).toHaveBeenCalledWith('primary', file, 0);
    expect(await screen.findByText('Primary logo uploaded.')).toBeInTheDocument();
  });
});
