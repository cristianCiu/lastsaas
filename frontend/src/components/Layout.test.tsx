import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Layout from './Layout';

const mocks = vi.hoisted(() => ({ isRootTenant: false, resolvedLocationBranding: null as { displayName: string } | null }));

vi.mock('../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'user-a' }, isAuthenticated: true, logout: vi.fn(), memberships: [] }) }));
vi.mock('../contexts/TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: 'tenant-a', tenantName: 'Bistro' }, setActiveTenant: vi.fn(), isRootTenant: mocks.isRootTenant }) }));
vi.mock('../contexts/ActiveLocationContext', () => ({ useActiveLocation: () => ({ locations: [], loading: false, error: null, activeLocation: null, setActiveLocation: vi.fn() }) }));
vi.mock('../contexts/BrandingContext', () => ({ useBranding: () => ({ branding: { appName: 'Platform', logoMode: 'both', logoUrl: '/platform.png', navItems: [] } }) }));
vi.mock('../contexts/TenantBrandingContext', () => ({ useTenantBranding: () => ({ primaryLogoUrl: 'blob:primary', compactLogoUrl: 'blob:compact', resolvedLocationBranding: mocks.resolvedLocationBranding }) }));
vi.mock('../contexts/ThemeContext', () => ({ useTheme: () => ({ resolvedTheme: 'dark', setTheme: vi.fn() }) }));
vi.mock('../api/client', () => ({
  messagesApi: { unreadCount: vi.fn().mockResolvedValue({ count: 0 }) },
  plansApi: { list: vi.fn().mockResolvedValue({ plans: [], tenantSubscriptionCredits: 0, tenantPurchasedCredits: 0, maxPlanUserLimit: 1 }) },
  bundlesApi: { list: vi.fn().mockResolvedValue({ bundles: [] }) },
  announcementsApi: { list: vi.fn().mockResolvedValue({ announcements: [] }) },
}));
vi.mock('./ImpersonationBanner', () => ({ default: () => null }));

describe('Layout tenant logos', () => {
  beforeEach(() => { mocks.isRootTenant = false; mocks.resolvedLocationBranding = null; });

  it('uses scoped primary and compact logos for a restaurant workspace', () => {
    render(<MemoryRouter><Layout /></MemoryRouter>);
    const logos = screen.getAllByAltText('Bistro');
    expect(logos).toHaveLength(2);
    expect(logos[0]).toHaveAttribute('src', 'blob:compact');
    expect(logos[1]).toHaveAttribute('src', 'blob:primary');
    expect(screen.getByText('Bistro')).toBeInTheDocument();
  });

  it('uses the resolved location display name for workspace identity', () => {
    mocks.resolvedLocationBranding = { displayName: 'Flagship' };
    render(<MemoryRouter><Layout /></MemoryRouter>);
    expect(screen.getAllByAltText('Flagship')).toHaveLength(2);
    expect(screen.getByText('Flagship')).toBeInTheDocument();
  });

  it('keeps the platform logo for the root workspace', () => {
    mocks.isRootTenant = true;
    render(<MemoryRouter><Layout /></MemoryRouter>);
    expect(screen.getByAltText('Platform')).toHaveAttribute('src', '/platform.png');
    expect(screen.queryByAltText('Bistro')).not.toBeInTheDocument();
  });
});
