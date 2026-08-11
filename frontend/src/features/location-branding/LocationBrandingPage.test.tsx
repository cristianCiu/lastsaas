import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import LocationBrandingPage from './LocationBrandingPage';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  update: vi.fn(),
  reset: vi.fn(),
}));

const resolved = {
  locationId: 'location-a', displayName: 'Main Dining', primaryColor: '#112233', accentColor: '#445566', font: 'system' as const,
  locationBrandingVersion: 2, tenantBrandingVersion: 4,
  sources: { displayName: 'location_branding', primaryColor: 'tenant', accentColor: 'tenant', font: 'platform' },
};
const existing = { displayName: 'Main Dining', primaryColor: '', accentColor: '', font: '' as const, version: 2 };

vi.mock('../../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'user-a' } }) }));
vi.mock('../../contexts/TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: 'tenant-a' }, role: 'owner', isRootTenant: false }) }));
vi.mock('../../contexts/ActiveLocationContext', () => ({
  useActiveLocation: () => ({
    activeLocation: { id: 'location-a', name: 'Main', code: 'MAIN', timezone: 'Europe/London', isActive: true },
    locations: [{ id: 'location-a', name: 'Main', code: 'MAIN', timezone: 'Europe/London', isActive: true }],
    loading: false,
  }),
}));
vi.mock('./api', () => ({ locationBrandingApi: { get: mocks.get, update: mocks.update, reset: mocks.reset } }));

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter><LocationBrandingPage /></MemoryRouter></QueryClientProvider>);
}

describe('LocationBrandingPage', () => {
  beforeEach(() => {
    mocks.get.mockReset().mockResolvedValue({ branding: existing, resolved, entitled: true });
    mocks.update.mockReset();
    mocks.reset.mockReset();
  });

  it('publishes normalized location overrides with the loaded version', async () => {
    const user = userEvent.setup();
    mocks.update.mockResolvedValue({ branding: { displayName: 'Patio', primaryColor: '#aabbcc', accentColor: '', font: '', version: 3 } });
    mocks.get.mockResolvedValueOnce({ branding: existing, resolved, entitled: true }).mockResolvedValue({
      branding: { displayName: 'Patio', primaryColor: '#aabbcc', accentColor: '', font: '', version: 3 },
      resolved: { ...resolved, displayName: 'Patio', primaryColor: '#aabbcc', locationBrandingVersion: 3 },
      entitled: true,
    });
    renderPage();

    const displayName = await screen.findByLabelText('Location display name');
    await user.clear(displayName);
    await user.type(displayName, ' Patio ');
    await user.type(screen.getByLabelText('Primary color'), '#AABBCC');
    await user.click(screen.getByRole('button', { name: 'Publish override' }));

    await waitFor(() => expect(mocks.update).toHaveBeenCalledWith('location-a', {
      displayName: 'Patio', primaryColor: '#aabbcc', accentColor: '', font: '', version: 2,
    }));
    expect(await screen.findByText('Location branding published.')).toBeInTheDocument();
  });

  it('removes the current override using optimistic versioning', async () => {
    const user = userEvent.setup();
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    mocks.reset.mockResolvedValue(undefined);
    renderPage();

    await screen.findByDisplayValue('Main Dining');
    await user.click(screen.getByRole('button', { name: 'Remove override' }));

    await waitFor(() => expect(mocks.reset).toHaveBeenCalledWith('location-a', 2));
    expect(await screen.findByText('Location override removed. Tenant branding now applies.')).toBeInTheDocument();
    vi.mocked(window.confirm).mockRestore();
  });
});
