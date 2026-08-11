# LastSaaS Version Notes

## v1.6.0 — August 11, 2026

### Exact Unit Catalog
- **Tenant-owned units** (`991d275`) — mass, volume, and count units with canonical tenant-unique codes, symbols, 0-6 display precision, optimistic versions, and soft lifecycle management.
- **Fixed-point quantity core** — signed `int64` micro-units with exact decimal parsing/formatting, checked add/subtract, rational conversion, overflow detection, and explicit rejection of inexact results.
- **Catalog authorization** — `catalog.read` and `catalog.manage` permissions follow active staff profiles, role defaults, and explicit allow/deny overrides.
- **Responsive management** — tenant-safe React Query caches and complete loading, empty, error, denied, read-only, create, edit, lifecycle, and conflict states.
- **Verification** — 68 frontend tests pass with build/lint; focused Atlas lifecycle, permission, tenant-isolation, unique-index, and strict-schema tests pass.

---

## v1.5.0 — August 11, 2026

### Guided Restaurant Onboarding
- **Tenant-scoped resumable setup** (`aec363f`) — restaurant company name/defaults and the first active location are persisted independently, server status reconstructs progress, and only owners can complete the audited, idempotent workflow.
- **Safe route enforcement** — incomplete owners enter onboarding automatically, completed owners cannot re-enter it, root/member workspaces bypass it, and plan/billing recovery remains accessible even when billing is inactive.
- **Failure-safe identity provisioning** — password and OAuth registration no longer issue sessions after failed tenant/membership/staff-profile provisioning; partial user records are removed.

### Entitlement And Isolation Hardening
- Known plan contracts require numeric `max_locations >= 1` and boolean `location_branding`; malformed entitlement types no longer pass boolean feature checks.
- Plan downgrades preserve location-branding records but exclude them from resolved workspace branding until the entitlement is restored.
- Location-branding ETags include location, override, tenant-branding, and entitlement state; tenant-branding changes invalidate resolved location caches.
- Product mutations require active staff profiles, and location updates enforce assigned-location access with non-leaking not-found behavior.

### Verification
- All 64 frontend tests across 20 files pass with lint, type-check, and production build.
- All Atlas-backed backend packages pass, including authenticated onboarding, entitlement allow/deny/downgrade, staff assignment/status, audit idempotency, and cross-tenant isolation coverage.
- The config unit package passes with the external `SERVER_PORT` test override unset; authenticated live-browser verification still requires a running seeded backend.

---

## v1.4.0 — August 11, 2026

### Restaurant Location Branding
- **Resolved location branding API** (`caaba7e`) — safe location overrides with assigned-location authorization, `location_branding` entitlement enforcement, owner/admin writes, optimistic concurrency, private ETags, reset auditing, and deterministic location → tenant → platform fallback.
- **Active-location branding frontend** (`4c8cf1f`) — active-location colors and fonts drive authenticated workspace themes, resolved display names identify the workspace, and public/root surfaces retain platform branding.
- **Location branding self-service** (`4c8cf1f`) — responsive settings editor with inherited-value preview, source visibility, normalized publish, versioned reset, conflict recovery, and explicit read-only, root, empty, loading, and error states.
- **Verification** — production frontend build and lint pass; all 57 frontend tests across 18 files pass; Atlas-backed backend coverage passed with the API slice.

### Restaurant Foundation Carried Forward
- Tenant-isolated locations, restaurant settings, storage areas, active-location selection, staff authorization, restricted tenant branding, and authenticated tenant logos remain the completed Phase 1 foundation.

---

## v1.3 — March 5, 2026

### SaaS Boilerplate Baseline
- Original LastSaaS baseline for this restaurant-product fork, including authentication, multi-tenancy, Stripe billing, administration, platform branding, API keys, outgoing webhooks, telemetry, health monitoring, and deployment infrastructure.
- Added DataDog metric corrections and a Today product-analytics time filter in `c692923`.

### Restaurant Product Work Initially Developed on v1.3
- Tenant-scoped locations and active-location selection (`d36716d`).
- Restaurant settings and location-scoped storage areas (`7efeeaf`).
- Staff profiles, business permissions, and assigned-location authorization (`b47ddd4`).
- Restricted tenant branding and isolated asset APIs (`8fb1dc1`).
- Authenticated tenant logo management and rendering (`fca14d1`).
- Stabilized Atlas integration infrastructure (`2a2bc9d`).

---

## v1.2 — March 1, 2026

### Product Analytics & Telemetry (New)
- **Conversion funnel dashboard** — visualize the customer journey from visitor to paid subscriber with conversion rates at each step (Visitors → Signups → Plan Page Views → Checkouts → Paid Conversions → Upgrades)
- **SaaS KPIs** — MRR, ARR, ARPU, LTV, churn rate, trial-to-paid conversion rate, median time to first purchase, active subscriber count with trend sparklines
- **Retention cohort analysis** — weekly or monthly cohort retention heatmap tracking user engagement over time
- **Engagement metrics** — DAU/WAU/MAU for paying subscribers, average sessions per user, top features by usage, credit consumption trend
- **Custom event explorer** — browse all telemetry event types, view trend charts, filter by name and time range
- **Telemetry Go SDK** — `telemetry.Track()`, `TrackBatch()`, `TrackPageView()`, `TrackCheckoutStarted()`, `TrackLogin()` for zero-overhead in-process event recording
- **Telemetry REST API** — anonymous endpoint for page views (rate-limited at 60/min per IP) and authenticated endpoints for custom events (120/min per user)
- **Auto-instrumentation** — registration, email verification, login, checkout, subscription activation/cancellation, and plan changes tracked automatically with no configuration
- **365-day retention** with MongoDB TTL auto-expiration

### CI/CD & Testing (New)
- **GitHub Actions CI workflow** with Go build, lint, and test against a MongoDB service container
- **Codecov integration** with coverage badges (Stripe tests at 89.7% coverage)
- **Comprehensive backend test suite** — new tests across auth, middleware, Stripe, webhooks, events, models, validation, and version packages
- **Hybrid validation** — Go struct tag validation via `go-playground/validator` plus MongoDB JSON Schema enforcement across 15 collections
- Frontend test setup with Vitest

### MCP Server Improvements
- Converted from mixed read/write to **32 read-only tools** for safer AI-powered admin access (removed 5 write tools, added 16 new read-only tools including 6 PM/telemetry tools)
- Added **MCP registry manifests** and GoReleaser distribution for discoverability and easy installation
- **6 PM/telemetry tools** — `get_funnel`, `get_kpis`, `get_retention`, `get_engagement`, `get_custom_events`, `list_event_types`
- New tool categories: About, Health Metrics, Entitlement Keys, Credit Bundles, Root Members, Webhook management, PM/Telemetry

### Security Hardening
- **Timing-safe auth** — dummy bcrypt comparison on failed login to prevent account enumeration
- **Rate limit hardening** — switched IP detection from spoofable `X-Forwarded-For` to trusted `Fly-Client-IP` header; tightened MFA challenge limit from 5 to 3 attempts
- **Password reset tokens** — hashed storage (was plaintext), reduced expiry from 60 to 30 minutes, previous unused tokens revoked on new request
- **Session revocation on password change** — all sessions invalidated when password is updated
- **Billing abuse prevention** — trial abuse detection across both tenant and user history; Stripe Customer ID cross-referencing to prevent subscription reassignment; atomic webhook processing to prevent race conditions
- **Refund and dispute handling** — new webhook handlers for `charge.refunded`, `charge.dispute.created`, `charge.dispute.closed`
- **Webhook secrets encrypted at rest** with AES-256-GCM
- **NoSQL injection protection** — user input escaped in all MongoDB `$regex` queries across search endpoints
- **XSS fix** — DOMPurify sanitization for branding HTML injection; fixed XSS vulnerability in email fallback templates
- **CSV injection protection** — all CSV exports sanitized against formula injection
- **Scoped logout** — token revocation scoped to authenticated user (was previously unscoped)
- **Impersonation tightened** — token window reduced from 15 to 5 minutes
- **MFA recovery codes** — increased entropy from 5 to 16 bytes
- **Request body size limit** — 1MB cap on all API routes

### Infrastructure & Quality
- **OpenAPI 3.0 spec** served at `/api/docs` as JSON
- **Structured API errors** — machine-readable error codes with request ID for traceability
- **Request ID middleware** — unique `X-Request-ID` header on every response
- **API version header** — `X-API-Version` on all responses
- **Server-side app name injection** into index.html (eliminates title flicker on page load)
- **Structured logging** — migrated from `log.Printf` to `log/slog` across all backend packages
- **Batch query optimization** — replaced N+1 queries in admin user deletion with `$in` batch fetches
- **Reusable UI component library** — standardized Alert, Badge, Button, Card, Input, Modal, Select, Textarea primitives
- **Send Test Email** button on health dashboard for Resend integration verification

---

## v1.0 — February 25, 2026

### Initial Public Release
- Multi-tenant architecture with role-based access control (owner/admin/user)
- Three-tier admin access: user (read-only), admin (read-write), owner (destructive)
- Root Members management for the admin team
- Email/password authentication with bcrypt hashing and JWT tokens
- MFA/TOTP two-factor authentication with setup wizard and recovery codes
- Magic link passwordless login via email
- Google, GitHub, and Microsoft OAuth with automatic account linking
- Passkey/WebAuthn support for passwordless authentication
- Session management with individual and bulk session revocation
- Dark/light theme preference per user
- Email verification via Resend
- Account lockout after failed login attempts
- Stripe Checkout integration for subscription billing
- Stripe Billing Portal for customer self-service
- Per-seat pricing model with included seats, min/max seat limits
- Free trials with configurable trial days per plan
- Stripe Tax integration for automatic tax calculation
- Promotion codes and coupons with expiration dates and product restrictions
- Credit bundles for one-time credit purchases
- PDF invoice generation with company name, address, and tax breakdown
- Multi-currency support with configurable default currency
- Plan management with entitlements (boolean and numeric)
- Billing enforcement middleware
- Dual credit buckets (subscription + purchased) with configurable reset policies
- Team invitations and member management
- Ownership transfer between members
- Per-tenant activity logs
- User profile management and account deletion
- White-label branding: custom app name, tagline, logo, theme colors, fonts, favicon, media library, custom landing page, custom pages, CSS/HTML injection, configurable nav sidebar, auth page customization, dashboard HTML, Open Graph images
- Admin dashboard with user and tenant management
- Admin impersonation for debugging
- Financial dashboard with revenue, ARR, DAU, MAU time-series charting
- Onboarding flow for new users
- System-wide announcements
- In-app messaging to individual users
- CSV export for users and tenants
- System health monitoring with automatic node registration, 30-second heartbeat, metrics collection (CPU, memory, disk, network, HTTP, MongoDB, Go runtime), threshold-based alerting, real-time dashboard with time-series charts, integration health panel
- `lsk_`-prefixed API keys with admin and user authority scopes (SHA-256 hashed)
- Outgoing webhooks with 19 event types, HMAC-SHA256 signing, delivery tracking, test events
- MCP server with 16 tools for AI-powered admin access
- Built-in API documentation at `/api/docs` with interactive HTML and markdown references
- Configuration variable editor (strings, numbers, enums, templates)
- System logging with injection detection
- Security headers (CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy)
- Rate limiting on authentication endpoints
- Refresh token rotation with family-based revocation
- CLI tools: `setup`, `start`/`stop`/`restart`, `change-password`, `send-message`, `transfer-root-owner`, `config`, `version`, `status`, `mcp`
- Dockerized deployment (Go + React + Alpine)
- Fly.io deployment configuration
- Graceful shutdown with connection draining
- Compile-time version embedding via ldflags
- Auto-versioning with database migration on startup
