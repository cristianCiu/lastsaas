import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import OnboardingGate from './OnboardingGate';

const mocks = vi.hoisted(() => ({ completed: false, role: 'owner' as 'owner' | 'admin' | 'user', isRootTenant: false, get: vi.fn() }));

vi.mock('../../contexts/AuthContext', () => ({ useAuth: () => ({ user: { id: 'user-a' }, memberships: [{ tenantId: 'tenant-a' }] }) }));
vi.mock('../../contexts/TenantContext', () => ({ useTenant: () => ({ activeTenant: { tenantId: 'tenant-a' }, role: mocks.role, isRootTenant: mocks.isRootTenant }) }));
vi.mock('./api', () => ({ onboardingApi: { get: mocks.get } }));

function renderRoute(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[path]}><Routes><Route element={<OnboardingGate />}><Route path="/dashboard" element={<div>Dashboard</div>} /><Route path="/onboarding" element={<div>Onboarding</div>} /><Route path="/plan" element={<div>Plan recovery</div>} /></Route></Routes></MemoryRouter></QueryClientProvider>);
}

describe('OnboardingGate', () => {
  beforeEach(() => {
    mocks.completed = false;
    mocks.role = 'owner';
    mocks.isRootTenant = false;
    mocks.get.mockReset().mockImplementation(() => Promise.resolve({ onboarding: { completed: mocks.completed } }));
  });

  it('redirects an incomplete restaurant owner into onboarding', async () => {
    renderRoute('/dashboard');
    expect(await screen.findByText('Onboarding')).toBeInTheDocument();
  });

  it('redirects a completed owner away from onboarding', async () => {
    mocks.completed = true;
    renderRoute('/onboarding');
    expect(await screen.findByText('Dashboard')).toBeInTheDocument();
  });

  it('does not run owner onboarding for regular members', async () => {
    mocks.role = 'user';
    renderRoute('/dashboard');
    expect(await screen.findByText('Dashboard')).toBeInTheDocument();
    expect(mocks.get).not.toHaveBeenCalled();
  });
});
