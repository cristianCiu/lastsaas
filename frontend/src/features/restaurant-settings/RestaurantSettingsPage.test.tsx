import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import RestaurantSettingsPage from './RestaurantSettingsPage';

const { getSettings } = vi.hoisted(() => ({ getSettings: vi.fn() }));

vi.mock('./api', () => ({ restaurantSettingsApi: { get: getSettings, update: vi.fn() } }));
vi.mock('../../contexts/TenantContext', () => ({
  useTenant: () => ({ activeTenant: { tenantId: 'tenant-a', tenantName: 'Bistro' }, role: 'admin' }),
}));

describe('RestaurantSettingsPage', () => {
  it('renders the settings property from the API response contract', async () => {
    getSettings.mockResolvedValueOnce({
      settings: { currency: 'USD', language: 'en-US', defaultTimezone: 'America/New_York', version: 4 },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/settings/restaurant']}><RestaurantSettingsPage /></MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByDisplayValue('USD')).toBeInTheDocument();
    expect(screen.getByDisplayValue('en-US')).toBeInTheDocument();
    expect(screen.getByDisplayValue('America/New_York')).toBeInTheDocument();
    expect(screen.getByText('Saved with version 4')).toBeInTheDocument();
  });
});
