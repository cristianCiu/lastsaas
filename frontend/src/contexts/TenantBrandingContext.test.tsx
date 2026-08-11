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
  getLocationBranding: vi.fn(),
  activeLocationId: 'location-a' as string | null,
  createObjectURL: vi.fn(),
  revokeObjectURL: vi.fn(),
}));

vi.mock('./AuthContext', () => ({ useAuth: () => ({ user: { id: mocks.principalId }, isAuthenticated: true }) }));
vi.mock('./TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: mocks.tenantId }, isRootTenant: false }) }));
vi.mock('./ActiveLocationContext', () => ({ useActiveLocation: () => ({ activeLocation: mocks.activeLocationId ? { id: mocks.activeLocationId } : null }) }));
vi.mock('../features/tenant-branding/api', () => ({ tenantBrandingApi: { get: mocks.get, listAssets: mocks.listAssets, getAsset: mocks.getAsset } }));
vi.mock('../features/location-branding/api', () => ({ locationBrandingApi: { get: mocks.getLocationBranding } }));

function Probe() {
  const { primaryLogoUrl, compactLogoUrl, assets, effectiveBranding, resolvedLocationBranding } = useTenantBranding();
  return <div>{primaryLogoUrl ?? 'no-primary'}|{compactLogoUrl ?? 'no-compact'}|{assets.length}|{effectiveBranding?.primaryColor || 'no-color'}|{resolvedLocationBranding?.displayName || 'no-location'}</div>;
}

function tree(client: QueryClient) {
  return <QueryClientProvider client={client}><TenantBrandingProvider><Probe /></TenantBrandingProvider></QueryClientProvider>;
}

describe('TenantBrandingProvider assets', () => {
  afterEach(() => vi.unstubAllGlobals());
  beforeEach(() => {
    mocks.tenantId = 'tenant-a';
    mocks.principalId = 'user-a';
    mocks.activeLocationId = 'location-a';
    mocks.get.mockReset().mockResolvedValue({ branding: { primaryColor: '', accentColor: '', font: '', version: 0 } });
    mocks.listAssets.mockReset().mockResolvedValue({ assets: [{ id: 'asset-a', kind: 'primary', contentType: 'image/png', size: 100, width: 100, height: 50, version: 1, createdAt: '', updatedAt: '' }] });
    mocks.getAsset.mockReset().mockResolvedValue(new Blob(['logo'], { type: 'image/png' }));
    mocks.getLocationBranding.mockReset().mockResolvedValue({
      branding: { displayName: '', primaryColor: '', accentColor: '', font: '', version: 0 },
      resolved: { locationId: 'location-a', displayName: 'Flagship', primaryColor: '#123456', accentColor: '#654321', font: 'humanist', locationBrandingVersion: 0, tenantBrandingVersion: 0, sources: {} },
    });
    mocks.createObjectURL.mockReset().mockReturnValue('blob:tenant-a');
    mocks.revokeObjectURL.mockReset();
    vi.stubGlobal('URL', { createObjectURL: mocks.createObjectURL, revokeObjectURL: mocks.revokeObjectURL });
  });

  it('creates and revokes an authenticated logo URL without exposing it across tenant changes', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const view = render(tree(client));
    expect(await screen.findByText('blob:tenant-a|no-compact|1|#123456|Flagship')).toBeInTheDocument();
    expect(mocks.getAsset).toHaveBeenCalledWith('primary');
    expect(mocks.getLocationBranding).toHaveBeenCalledWith('location-a');

    mocks.tenantId = 'tenant-b';
    mocks.listAssets.mockResolvedValueOnce({ assets: [] });
    view.rerender(tree(client));
    expect(screen.getByText('no-primary|no-compact|0|no-color|no-location')).toBeInTheDocument();
    await waitFor(() => expect(mocks.revokeObjectURL).toHaveBeenCalledWith('blob:tenant-a'));
  });

  it('falls back to tenant branding when no active location exists', async () => {
    mocks.activeLocationId = null;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(tree(client));
    expect(await screen.findByText('blob:tenant-a|no-compact|1|no-color|no-location')).toBeInTheDocument();
    expect(mocks.getLocationBranding).not.toHaveBeenCalled();
  });
});
