import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import OnboardingPage from './OnboardingPage';

const mocks = vi.hoisted(() => ({
  refreshUser: vi.fn(), updateTenant: vi.fn(), getSettings: vi.fn(), updateSettings: vi.fn(), createLocation: vi.fn(), getOnboarding: vi.fn(), complete: vi.fn(),
  locations: [] as Array<{ id: string; name: string; isActive: boolean }>, locationsLoading: false, locationsError: null as unknown,
}));

vi.mock('../../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'user-a' }, refreshUser: mocks.refreshUser }) }));
vi.mock('../../contexts/TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: 'tenant-a', tenantName: 'Old Team' } }) }));
vi.mock('../../contexts/ActiveLocationContext', () => ({ useActiveLocation: () => ({ locations: mocks.locations, loading: mocks.locationsLoading, error: mocks.locationsError }) }));
vi.mock('../../api/client', () => ({ tenantApi: { updateSettings: mocks.updateTenant } }));
vi.mock('../../features/restaurant-settings/api', () => ({ restaurantSettingsApi: { get: mocks.getSettings, update: mocks.updateSettings } }));
vi.mock('../../features/locations/api', () => ({ locationsApi: { create: mocks.createLocation } }));
vi.mock('../../features/onboarding/api', () => ({ onboardingApi: { get: mocks.getOnboarding, complete: mocks.complete } }));

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={['/onboarding']}><Routes><Route path="/onboarding" element={<OnboardingPage />} /><Route path="/dashboard" element={<div>Dashboard ready</div>} /></Routes></MemoryRouter></QueryClientProvider>);
}

describe('OnboardingPage', () => {
  beforeEach(() => {
    mocks.refreshUser.mockReset().mockResolvedValue(undefined);
    mocks.updateTenant.mockReset().mockResolvedValue({});
    mocks.getSettings.mockReset().mockResolvedValue({ settings: { currency: 'EUR', language: 'de-DE', defaultTimezone: 'Europe/Berlin', version: 0 } });
    mocks.updateSettings.mockReset().mockResolvedValue({ settings: { currency: 'EUR', language: 'de-DE', defaultTimezone: 'Europe/Berlin', version: 1 } });
    mocks.createLocation.mockReset().mockResolvedValue({ location: { id: 'location-a' } });
    mocks.getOnboarding.mockReset().mockResolvedValue({ onboarding: { completed: false, restaurantSettingsComplete: false, firstLocationComplete: false } });
    mocks.complete.mockReset().mockResolvedValue({ onboarding: { completed: true, restaurantSettingsComplete: true, firstLocationComplete: true } });
    mocks.locations = [];
    mocks.locationsLoading = false;
    mocks.locationsError = null;
  });

  it('persists restaurant defaults and a first location before completing', async () => {
    const user = userEvent.setup();
    renderPage();
    const name = await screen.findByLabelText('Restaurant company name');
    await user.clear(name);
    await user.type(name, 'Bistro One');
    await user.click(screen.getByRole('button', { name: 'Save and continue' }));

    await waitFor(() => expect(mocks.updateTenant).toHaveBeenCalledWith({ name: 'Bistro One' }));
    expect(mocks.updateSettings).toHaveBeenCalledWith({ currency: 'EUR', language: 'de-DE', defaultTimezone: 'Europe/Berlin', version: 0 });

    await user.type(await screen.findByLabelText('Location code'), 'main');
    await user.type(screen.getByLabelText('Location name'), 'Main Restaurant');
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    await waitFor(() => expect(mocks.createLocation).toHaveBeenCalledWith({ code: 'main', name: 'Main Restaurant', timezone: 'Europe/Berlin' }));

    await user.click(await screen.findByRole('button', { name: 'Complete setup' }));
    expect(await screen.findByText('Dashboard ready')).toBeInTheDocument();
    expect(mocks.complete).toHaveBeenCalledTimes(1);
  });

  it('does not leave onboarding when completion fails', async () => {
    const user = userEvent.setup();
    mocks.complete.mockRejectedValue(new Error('Completion failed'));
    renderPage();
    await screen.findByLabelText('Restaurant company name');
    await user.click(screen.getByRole('button', { name: 'Save and continue' }));
    await screen.findByLabelText('Location code');
    await user.type(screen.getByLabelText('Location code'), 'main');
    await user.type(screen.getByLabelText('Location name'), 'Main Restaurant');
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    await user.click(await screen.findByRole('button', { name: 'Complete setup' }));
    expect(await screen.findByText('Completion failed')).toBeInTheDocument();
    expect(screen.queryByText('Dashboard ready')).not.toBeInTheDocument();
  });

  it('resumes at completion without creating a duplicate location', async () => {
    const user = userEvent.setup();
    mocks.getOnboarding.mockResolvedValue({ onboarding: { completed: false, restaurantSettingsComplete: true, firstLocationComplete: true } });
    mocks.locations = [{ id: 'location-a', name: 'Existing', isActive: true }];
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Complete setup' }));
    expect(await screen.findByText('Dashboard ready')).toBeInTheDocument();
    expect(mocks.createLocation).not.toHaveBeenCalled();
  });

  it('returns to restaurant basics when only a location is persisted', async () => {
    mocks.getOnboarding.mockResolvedValue({ onboarding: { completed: false, restaurantSettingsComplete: false, firstLocationComplete: true } });
    renderPage();
    expect(await screen.findByLabelText('Restaurant company name')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Complete setup' })).not.toBeInTheDocument();
  });
});
