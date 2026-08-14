import { lazy, Suspense, useEffect, useState } from 'react';
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Toaster } from 'sonner';
import { AuthProvider } from './contexts/AuthContext';
import { TenantProvider } from './contexts/TenantContext';
import { BrandingProvider } from './contexts/BrandingContext';
import { ThemeProvider } from './contexts/ThemeContext';
import { TenantBrandingProvider } from './contexts/TenantBrandingContext';
import Layout from './components/Layout';
import AdminLayout from './components/AdminLayout';
import ProtectedRoute from './components/ProtectedRoute';
import BrandingThemeInjector from './components/BrandingThemeInjector';
import LoadingSpinner from './components/LoadingSpinner';
import ErrorBoundary from './components/ErrorBoundary';
import { bootstrapApi } from './api/client';

// Auth pages (lazy — not needed until user navigates to them)
const LoginPage = lazy(() => import('./pages/auth/LoginPage'));
const SignupPage = lazy(() => import('./pages/auth/SignupPage'));
const VerifyEmailPage = lazy(() => import('./pages/auth/VerifyEmailPage'));
const ForgotPasswordPage = lazy(() => import('./pages/auth/ForgotPasswordPage'));
const ResetPasswordPage = lazy(() => import('./pages/auth/ResetPasswordPage'));
const AuthCallbackPage = lazy(() => import('./pages/auth/AuthCallbackPage'));
const MFAChallengePage = lazy(() => import('./pages/auth/MFAChallengePage'));
const MagicLinkVerifyPage = lazy(() => import('./pages/auth/MagicLinkVerifyPage'));
import BootstrapPage from './pages/BootstrapPage';

// App pages (eager — core experience)
import DashboardPage from './pages/app/DashboardPage';
import TeamPage from './pages/app/TeamPage';
import SettingsPage from './pages/app/SettingsPage';
import PlanPage from './pages/app/PlanPage';
import BuyCreditsPage from './pages/app/BuyCreditsPage';
import BillingSuccessPage from './pages/app/BillingSuccessPage';
import BillingCancelPage from './pages/app/BillingCancelPage';
import TestEntitlementsPage from './pages/app/TestEntitlementsPage';
import ActivityPage from './pages/app/ActivityPage';
import OnboardingPage from './pages/app/OnboardingPage';
import LocationsPage from './features/locations/LocationsPage';
import RestaurantSettingsPage from './features/restaurant-settings/RestaurantSettingsPage';
import StorageAreasPage from './features/storage-areas/StorageAreasPage';
import TenantBrandingPage from './features/tenant-branding/TenantBrandingPage';
import LocationBrandingPage from './features/location-branding/LocationBrandingPage';
import OnboardingGate from './features/onboarding/OnboardingGate';
import UnitsPage from './features/master-data/UnitsPage';
import CategoriesPage from './features/master-data/CategoriesPage';
import ItemsPage from './features/master-data/ItemsPage';
import ConversionsPage from './features/master-data/ConversionsPage';
import SuppliersPage from './features/master-data/SuppliersPage';
import SupplierTermsPage from './features/master-data/SupplierTermsPage';
import ImportsPage from './features/imports/ImportsPage';
import InventoryPage from './features/inventory/InventoryPage';
import RecipesPage from './features/recipes/RecipesPage';
import MappingsPage from './features/recipes/MappingsPage';
import SalesImportsPage from './features/sales/SalesImportsPage';
import UnmappedSalesPage from './features/sales/UnmappedSalesPage';
import PurchaseOrdersPage, { NewOrderPage } from './features/purchasing/PurchaseOrdersPage';
import CalendarPage from './features/purchasing/CalendarPage';
import ReceiptsPage, { ReceivePage } from './features/purchasing/ReceiptsPage';

// Admin pages (lazy — only loaded by root tenant admins)
const AdminDashboardPage = lazy(() => import('./pages/admin/DashboardPage'));
const AdminMessagesPage = lazy(() => import('./pages/admin/MessagesPage'));
const AdminUsersPage = lazy(() => import('./pages/admin/UsersPage'));
const AdminTenantsPage = lazy(() => import('./pages/admin/TenantsPage'));
const AdminLogsPage = lazy(() => import('./pages/admin/LogsPage'));
const AdminConfigPage = lazy(() => import('./pages/admin/ConfigPage'));
const AdminUserProfilePage = lazy(() => import('./pages/admin/UserProfilePage'));
const AdminAboutPage = lazy(() => import('./pages/admin/AboutPage'));
const AdminPlansPage = lazy(() => import('./pages/admin/PlansPage'));
const AdminTenantProfilePage = lazy(() => import('./pages/admin/TenantProfilePage'));
const AdminHealthPage = lazy(() => import('./pages/admin/HealthPage'));
const AdminFinancialPage = lazy(() => import('./pages/admin/FinancialPage'));
const AdminAPIPage = lazy(() => import('./pages/admin/APIPage'));
const AdminBrandingPage = lazy(() => import('./pages/admin/BrandingPage'));
const AdminPromotionsPage = lazy(() => import('./pages/admin/PromotionsPage'));
const AdminAnnouncementsPage = lazy(() => import('./pages/admin/AnnouncementsPage'));
const AdminRootMembersPage = lazy(() => import('./pages/admin/RootMembersPage'));
const AdminPMPage = lazy(() => import('./pages/admin/PMPage'));

// Public pages
import LandingPage from './pages/public/LandingPage';
import CustomPage from './pages/public/CustomPage';

function LazyFallback() {
  return (
    <div className="flex items-center justify-center py-20">
      <LoadingSpinner size="lg" />
    </div>
  );
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60,
      retry: 1,
    },
  },
});

function ScrollToTop() {
  const { pathname } = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [pathname]);
  return null;
}

function BootstrapGuard({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<'loading' | 'initialized' | 'needs-setup'>('loading');

  useEffect(() => {
    bootstrapApi.status()
      .then((data) => setStatus(data.initialized ? 'initialized' : 'needs-setup'))
      .catch(() => setStatus('initialized')); // If bootstrap endpoint fails, assume initialized
  }, []);

  if (status === 'loading') {
    return (
      <div className="min-h-screen bg-dark-950 flex items-center justify-center">
        <LoadingSpinner size="lg" />
      </div>
    );
  }

  if (status === 'needs-setup') {
    return (
      <BrowserRouter>
        <Routes>
          <Route path="/setup" element={<BootstrapPage />} />
          <Route path="*" element={<Navigate to="/setup" replace />} />
        </Routes>
      </BrowserRouter>
    );
  }

  return <>{children}</>;
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BootstrapGuard>
        <BrandingProvider>
          <AuthProvider>
            <ThemeProvider>
              <TenantProvider>
                <TenantBrandingProvider>
                  <BrowserRouter>
                  <ScrollToTop />
                  <BrandingThemeInjector />
                  <ErrorBoundary>
                  <Routes>
                    {/* Public landing page */}
                    <Route path="/" element={<LandingPage />} />

                    {/* Public custom pages */}
                    <Route path="/p/:slug" element={<CustomPage />} />

                    {/* Public auth routes (lazy-loaded) */}
                    <Route path="/login" element={<Suspense fallback={<LazyFallback />}><LoginPage /></Suspense>} />
                    <Route path="/signup" element={<Suspense fallback={<LazyFallback />}><SignupPage /></Suspense>} />
                    <Route path="/verify-email" element={<Suspense fallback={<LazyFallback />}><VerifyEmailPage /></Suspense>} />
                    <Route path="/forgot-password" element={<Suspense fallback={<LazyFallback />}><ForgotPasswordPage /></Suspense>} />
                    <Route path="/reset-password" element={<Suspense fallback={<LazyFallback />}><ResetPasswordPage /></Suspense>} />
                    <Route path="/auth/callback" element={<Suspense fallback={<LazyFallback />}><AuthCallbackPage /></Suspense>} />
                    <Route path="/auth/mfa" element={<Suspense fallback={<LazyFallback />}><MFAChallengePage /></Suspense>} />
                    <Route path="/auth/magic-link" element={<Suspense fallback={<LazyFallback />}><MagicLinkVerifyPage /></Suspense>} />

                    {/* Protected app routes */}
                    <Route element={<ProtectedRoute />}>
                      <Route element={<OnboardingGate />}>
                      {/* Onboarding (no layout) */}
                      <Route path="/onboarding" element={<OnboardingPage />} />

                      <Route element={<Layout />}>
                        <Route path="/dashboard" element={<DashboardPage />} />
                        <Route path="/team" element={<TeamPage />} />
                        <Route path="/plan" element={<PlanPage />} />
                        <Route path="/buy-credits" element={<BuyCreditsPage />} />
                        <Route path="/billing/success" element={<BillingSuccessPage />} />
                        <Route path="/billing/cancel" element={<BillingCancelPage />} />
                        <Route path="/settings" element={<SettingsPage />} />
                        <Route path="/settings/locations" element={<LocationsPage />} />
                        <Route path="/settings/restaurant" element={<RestaurantSettingsPage />} />
                        <Route path="/settings/branding" element={<TenantBrandingPage />} />
                        <Route path="/settings/location-branding" element={<LocationBrandingPage />} />
                        <Route path="/settings/storage-areas" element={<StorageAreasPage />} />
                        <Route path="/settings/master-data/units" element={<UnitsPage />} />
                        <Route path="/settings/master-data/categories" element={<CategoriesPage />} />
                        <Route path="/settings/master-data/items" element={<ItemsPage />} />
                        <Route path="/settings/master-data/conversions" element={<ConversionsPage />} />
                        <Route path="/settings/master-data/suppliers" element={<SuppliersPage />} />
                        <Route path="/settings/master-data/supplier-terms" element={<SupplierTermsPage />} />
                        <Route path="/settings/imports" element={<ImportsPage />} />
                        <Route path="/inventory" element={<InventoryPage />} />
                        <Route path="/inventory/journal" element={<InventoryPage />} />
                        <Route path="/inventory/lots" element={<InventoryPage />} />
                        <Route path="/inventory/counts" element={<InventoryPage />} />
                        <Route path="/inventory/reconciliation" element={<InventoryPage />} />
                        <Route path="/recipes" element={<RecipesPage />} />
                        <Route path="/recipes/:recipeId" element={<RecipesPage />} />
                        <Route path="/recipes/mappings" element={<MappingsPage />} />
                        <Route path="/sales/imports" element={<SalesImportsPage />} />
                        <Route path="/sales/imports/:runId" element={<SalesImportsPage />} />
                        <Route path="/sales/unmapped" element={<UnmappedSalesPage />} />
                        <Route path="/purchasing/orders" element={<PurchaseOrdersPage />} />
                        <Route path="/purchasing/orders/new" element={<NewOrderPage />} />
                        <Route path="/purchasing/orders/:orderId" element={<PurchaseOrdersPage />} />
                        <Route path="/purchasing/orders/:orderId/receive" element={<ReceivePage />} />
                        <Route path="/purchasing/calendar" element={<CalendarPage />} />
                        <Route path="/purchasing/receipts" element={<ReceiptsPage />} />
                        <Route path="/purchasing/receipts/:receiptId" element={<ReceiptsPage />} />
                        <Route path="/activity" element={<ActivityPage />} />
                        <Route path="/test-entitlements" element={<TestEntitlementsPage />} />
                        <Route path="/messages" element={<Suspense fallback={<LazyFallback />}><AdminMessagesPage /></Suspense>} />
                      </Route>
                      </Route>

                      {/* Admin routes (root tenant only, enforced by AdminLayout) */}
                      <Route path="/last" element={<AdminLayout />}>
                        <Route index element={<Suspense fallback={<LazyFallback />}><AdminDashboardPage /></Suspense>} />
                        <Route path="messages" element={<Suspense fallback={<LazyFallback />}><AdminMessagesPage /></Suspense>} />
                        <Route path="users" element={<Suspense fallback={<LazyFallback />}><AdminUsersPage /></Suspense>} />
                        <Route path="users/:userId" element={<Suspense fallback={<LazyFallback />}><AdminUserProfilePage /></Suspense>} />
                        <Route path="tenants" element={<Suspense fallback={<LazyFallback />}><AdminTenantsPage /></Suspense>} />
                        <Route path="tenants/:tenantId" element={<Suspense fallback={<LazyFallback />}><AdminTenantProfilePage /></Suspense>} />
                        <Route path="members" element={<Suspense fallback={<LazyFallback />}><AdminRootMembersPage /></Suspense>} />
                        <Route path="plans" element={<Suspense fallback={<LazyFallback />}><AdminPlansPage /></Suspense>} />
                        <Route path="financial" element={<Suspense fallback={<LazyFallback />}><AdminFinancialPage /></Suspense>} />
                        <Route path="pm" element={<Suspense fallback={<LazyFallback />}><AdminPMPage /></Suspense>} />
                        <Route path="promotions" element={<Suspense fallback={<LazyFallback />}><AdminPromotionsPage /></Suspense>} />
                        <Route path="announcements" element={<Suspense fallback={<LazyFallback />}><AdminAnnouncementsPage /></Suspense>} />
                        <Route path="health" element={<Suspense fallback={<LazyFallback />}><AdminHealthPage /></Suspense>} />
                        <Route path="logs" element={<Suspense fallback={<LazyFallback />}><AdminLogsPage /></Suspense>} />
                        <Route path="config" element={<Suspense fallback={<LazyFallback />}><AdminConfigPage /></Suspense>} />
                        <Route path="api" element={<Suspense fallback={<LazyFallback />}><AdminAPIPage /></Suspense>} />
                        <Route path="branding" element={<Suspense fallback={<LazyFallback />}><AdminBrandingPage /></Suspense>} />
                        <Route path="about" element={<Suspense fallback={<LazyFallback />}><AdminAboutPage /></Suspense>} />
                      </Route>
                    </Route>

                    {/* Fallback */}
                    <Route path="*" element={<Navigate to="/dashboard" replace />} />
                  </Routes>
                  </ErrorBoundary>
                  <Toaster
                    position="top-right"
                    toastOptions={{
                      style: {
                        background: 'var(--color-dark-900)',
                        border: '1px solid var(--color-dark-700)',
                        color: 'var(--color-dark-100)',
                      },
                    }}
                  />
                  </BrowserRouter>
                </TenantBrandingProvider>
              </TenantProvider>
            </ThemeProvider>
          </AuthProvider>
        </BrandingProvider>
      </BootstrapGuard>
    </QueryClientProvider>
  );
}
