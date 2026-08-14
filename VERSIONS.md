# LastSaaS Version Notes

## v5.0.0 — August 14, 2026

### Purchasing And Goods Receipt
- **Purchase orders** — location-scoped orders snapshot supplier terms, apply exact pack and MOQ rounding, require manager approval, and use delivery calendars.
- **Goods receipts** — partial receipts record quantity and price variances, lot and expiry data, and idempotent `goods_receipt` journal postings.
- **Order documents** — purchase orders render as tenant/location-branded PDFs. Managers can explicitly email a PDF attachment with idempotent delivery.
- **Purchasing UI** — responsive order, calendar, receipt, reversal, PDF download, and explicit document-email workflows use dedicated purchasing permissions.

### Verification
- `cd backend && go test ./...` and `cd backend && go build ./...` pass.
- `cd frontend && npx tsc --noEmit` passes.
- Receipt transaction coverage against a MongoDB replica set requires a configured `MONGODB_URI`.
- Frontend Vitest remains environment-blocked by fork-worker timeouts.

---

## v4.0.0 — August 14, 2026

### Recipes And Theoretical Consumption
- **Versioned recipes** — tenant-scoped recipes support immutable released versions, item and subrecipe components, fixed-point quantities, yields, losses, and effective dates.
- **POS mappings** — external product mappings select the released recipe version that was valid when the sale occurred.
- **Sales consumption** — bounded CSV imports support dry runs, idempotent applies, normalized sales lines, fixed-point recipe expansion, immutable `sale_consumption` postings, and cancellation reversals.
- **Operational UI** — responsive recipe, POS mapping, sales import, and unmapped-sales queue screens support the release workflow.
- **Unmapped sales** — unmapped products remain visible in a tenant-scoped queue. Correct the mapping, then import the sales data again.

### Verification
- `cd backend && go test ./...` and `cd backend && go build ./...` pass.
- `cd frontend && npx tsc --noEmit` passes.
- Replica-set transaction coverage requires a configured `MONGODB_URI`.
- Frontend Vitest remains environment-blocked by fork-worker timeouts.

---

## v3.0.0 — August 14, 2026

### Inventory Journal And Stock Counts
- **Append-only inventory journal** — location-scoped balances, postings, movements, lots, expiry, FEFO allocation, transfers, waste, reversals, and reconciliation are available in the Go modular monolith and React application.
- **Safe stock counts** — freeze-time storage-area fencing blocks conflicting inventory writes. Counts support discovered stock, cancellation, no-op receipts, exact idempotency, recovery after reload, legacy count quarantine, and safe count-line lot identity.
- **Scoped operations** — inventory references and count recovery use the required read/manage/post access without exposing journal or balance data to operational-only users.

### Verification
- Backend inventory, product, database, and validation tests pass. The Go build passes.
- Frontend TypeScript checking and the production build pass. The production build retains the existing main-bundle size warning.
- Replica-set transaction integration remains pending a configured `MONGODB_URI`.

---

## v2.0.0 — August 14, 2026

### Master-Data And Import Foundation
- **Master data** — the completed foundation covers tenant-scoped units, categories, items, canonical conversions, suppliers, and purchasing terms.
- **Bounded synchronous CSV imports** — UTF-8 comma/semicolon files support templates, canonical-header mapping, dry runs, transactional applies, idempotency, tenant-scoped runs, and bounded errors, with exact limits of 128 KiB per file and 5,000 rows per import.
- **Import wizard** — responsive workflow for template selection, column mapping, dry-run review, apply, run history, and errors.

### Verification
- Focused import/product/validation/handler tests and the Go build pass; frontend TypeScript checking and the production build pass.
- Full and focused Vitest verification remain environment-blocked by fork-worker timeouts.
- Replica-set integration tests require a separately configured test database. The configured `lastsaas-dev` database was correctly refused by the test safety guard. No live, Atlas, or Playwright verification is claimed.

---

## v1.10.0 — August 13, 2026

### Supplier And Purchasing-Term MVP
- **Tenant-wide suppliers** — suppliers have unique tenant-scoped codes, contact details, order days, and a default lead time.
- **Purchasing terms** — each tenant-wide supplier/item pair has one term with an exact decimal-string microquantity, minor-unit money, MOQ, and optional currency and lead-time overrides.
- **Safe references and lifecycle** — active same-tenant reference constraints, optimistic lifecycle versions, audit events, strict schemas/indexes, and tenant isolation cover suppliers and terms.
- **Responsive management** — supplier and purchasing-term UI supports tenant-safe supplier/item selection, exact quantity and money fields, and lifecycle operations.

### Verification
- Backend validation and handler tests plus the backend build pass.
- Frontend TypeScript checking and production build pass; Vitest remains environment-blocked by fork-worker timeouts.

---

## v1.9.0 — August 13, 2026

### Canonical Item Conversions
- **Tenant-scoped conversions** — each item has at most one conversion per source unit through the nested item API, with tenant-scoped uniqueness.
- **Exact factors** — source-to-item-base factors are positive reduced `int64` numerator/denominator pairs, each no greater than 1e9; creation requires active same-tenant source and base units with matching dimensions.
- **Safe lifecycle** — optimistic versions, audit events, strict schemas/indexes, and tenant isolation cover conversion lifecycle operations; item base-unit changes are blocked while conversions exist.
- **Responsive management** — standalone conversion UI supports item selection, source-unit options, exact previews, editing, and lifecycle operations.

### Verification
- Backend quantity, validation, and handler tests plus the backend build pass.
- Frontend TypeScript checking and production build pass; Vitest remains environment-blocked by fork-worker timeouts.

---

## v1.8.0 — August 13, 2026

### Item Master Data
- **Tenant-wide item identity** — items use immutable lowercase SKUs that are unique within the tenant.
- **Catalog references** — create and category/unit reference changes require active category and unit records from the same tenant; unrelated edits retain existing inactive references, while active replacements remain allowed.
- **Item attributes** — description, brand, and shelf-life days are optional; EU-14 allergen codes and stockable state are supported.
- **Lifecycle and isolation** — optimistic item versions and audit events cover lifecycle changes, with strict schemas/indexes and tenant-safe API and UI behavior.

### Verification
- `package-lock.json` is synchronized, `npm ci` works, and frontend TypeScript checking plus the production build pass.
- Focused validation tests passed earlier; the current Vitest run is blocked in this environment by fork-worker timeouts, so the frontend test suite is not claimed as passing.

---

## v1.7.0 — August 13, 2026

### Tenant Category Management
- **Flat tenant-wide categories** — categories are shared across all tenant locations and use canonical, tenant-unique slug codes.
- **Strict catalog persistence** — category documents use strict critical schema validation and tenant/code uniqueness indexes, with validation at both the API and model layers.
- **Catalog permissions and lifecycle** — `catalog.read` and `catalog.manage` enforcement covers category access; create, edit, deactivate, and reactivate operations use optimistic versions and tenant audit events.
- **Tenant-safe management UI** — responsive category management includes loading, empty, error, denied, read-only, mutation, lifecycle, conflict, and tenant-switch-safe cache states.

### Verification
- Backend category handler lifecycle/permission/isolation coverage, strict schema-rejection coverage, and model-validation coverage are present.
- Frontend category validation coverage is present; the package lock is now synchronized and full Vitest verification remains blocked by environment fork-worker timeouts.

---

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
