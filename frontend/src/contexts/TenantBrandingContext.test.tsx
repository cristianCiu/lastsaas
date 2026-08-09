import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TenantBrandingProvider, useTenantBranding } from './TenantBrandingContext';

const mocks = vi.hoisted(() => ({
  tenantId: 'tenant-a',
  principalId: 'user-a',
  get: vi.fn(),
  listAssets: vi.fn(),
  getAsset: vi.fn(),
  createObjectURL: vi.fn(),
  revokeObjectURL: vi.fn(),
}));

vi.mock('./AuthContext', () => ({ useAuth: () => ({ user: { id: mocks.principalId }, isAuthenticated: true }) }));
vi.mock('./TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: mocks.tenantId }, isRootTenant: false }) }));
vi.mock('../features/tenant-branding/api', () => ({ tenantBrandingApi: { get: mocks.get, listAssets: mocks.listAssets, getAsset: mocks.getAsset } }));

function Probe() {
  const { primaryLogoUrl, compactLogoUrl, assets } = useTenantBranding();
  return <div>{primaryLogoUrl ?? 'no-primary'}|{compactLogoUrl ?? 'no-compact'}|{assets.length}</div>;
}

function tree(client: QueryClient) {
  return <QueryClientProvider client={client}><TenantBrandingProvider><Probe /></TenantBrandingProvider></QueryClientProvider>;
}

describe('TenantBrandingProvider assets', () => {
  afterEach(() => vi.unstubAllGlobals());
  beforeEach(() => {
    mocks.tenantId = 'tenant-a';
    mocks.principalId = 'user-a';
    mocks.get.mockReset().mockResolvedValue({ branding: { primaryColor: '', accentColor: '', font: '', version: 0 } });
    mocks.listAssets.mockReset().mockResolvedValue({ assets: [{ id: 'asset-a', kind: 'primary', contentType: 'image/png', size: 100, width: 100, height: 50, version: 1, createdAt: '', updatedAt: '' }] });
    mocks.getAsset.mockReset().mockResolvedValue(new Blob(['logo'], { type: 'image/png' }));
    mocks.createObjectURL.mockReset().mockReturnValue('blob:tenant-a');
    mocks.revokeObjectURL.mockReset();
    vi.stubGlobal('URL', { createObjectURL: mocks.createObjectURL, revokeObjectURL: mocks.revokeObjectURL });
  });

  it('creates and revokes an authenticated logo URL without exposing it across tenant changes', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const view = render(tree(client));
    expect(await screen.findByText('blob:tenant-a|no-compact|1')).toBeInTheDocument();
    expect(mocks.getAsset).toHaveBeenCalledWith('primary');

    mocks.tenantId = 'tenant-b';
    mocks.listAssets.mockResolvedValueOnce({ assets: [] });
    view.rerender(tree(client));
    expect(screen.getByText('no-primary|no-compact|0')).toBeInTheDocument();
    await waitFor(() => expect(mocks.revokeObjectURL).toHaveBeenCalledWith('blob:tenant-a'));
  });
});
