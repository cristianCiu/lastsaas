# Implementation Status

## Current Phase

Phase 2: master data and import foundation.

## Completed

- Fork checkout configured with `origin` and `upstream` remotes.
- Go 1.25 and locked frontend dependencies installed for local development.
- Backend build and short-mode unit tests pass.
- Frontend TypeScript checking, lint, and production build pass; the current Vitest run is environment-blocked by fork-worker timeouts.
- Frontend dependencies updated within declared semver ranges; `npm audit` reports zero vulnerabilities.
- Direct-fork single-image deployment decision recorded and contradictory two-backend guidance removed.
- Product sequence refined to establish tenant/location security before safe tenant branding.
- Platform branding and tenant/location branding security boundary recorded.
- MongoDB replica-set and transaction requirement recorded.
- Tenant-isolated location list/create implemented across model, validation, strict schema, critical indexes, scoped repository, shared route registration, API documentation, audit logging, and frontend.
- Location creation enforces owner/admin roles and a concurrency-safe `max_locations` plan limit; tenants without a configured entitlement default to one location.
- Location update/deactivation enforces owner/admin roles, immutable codes, tenant-scoped optimistic concurrency, IANA timezone validation, and auditable lifecycle actions.
- Responsive location settings UI includes tenant-keyed caching, validation, loading, empty, error, read-only, and success states.
- Active-location selection is persisted per tenant, excludes inactive locations, resets safely across tenant/logout transitions, and is not loaded for the root platform tenant.
- Atlas-backed location tests pass for authentication, role enforcement, tenant isolation, hidden persistence fields, duplicate codes, default/numeric plan limits, and concurrent creation.
- Atlas-backed schema application succeeds and MongoDB rejects malformed location documents.
- Tenant-unique restaurant settings support virtual EUR/de/Europe/Berlin defaults, strict validated updates, optimistic concurrency, audit logging, and tenant-isolated caching.
- Location-scoped storage areas support strict types, unique names per location, create/update/deactivate/reactivate lifecycle operations, optimistic concurrency, audit logging, and tenant/location isolation.
- Storage-area APIs remain owner/admin-only until staff profiles can enforce independent location authorization.
- Strict critical MongoDB schemas and indexes cover restaurant settings and storage areas, including rejection of whitespace-only names and invalid enum values.
- Responsive restaurant and storage-area settings pages include workspace navigation, active-location scoping, read-only and empty states, version-conflict recovery, and mutation race isolation.
- Frontend verification now covers 30 tests across eight files; lint and the production build pass.
- Staff profiles extend tenant memberships with separate restaurant business roles, active status, all-location or explicit location assignments, and versioned permission overrides.
- Startup reconciliation creates conservative defaults for missing non-root profiles while validating existing profiles and failing closed on malformed authorization data.
- A critical partial unique index and transactional ownership workflows enforce exactly one tenant owner under concurrent transfers.
- Product authorization now enforces active staff profile, indistinguishable tenant/location scope, and explicit business permission checks before storage operations.
- Location lists are filtered by the caller's staff assignment; storage read and manage permissions are resolved independently with explicit overrides taking precedence.
- Membership creation, invitation acceptance, removal, ownership transfer, account deletion, and administrative deletion keep staff profiles transactionally consistent.
- Responsive Team management separates workspace roles from restaurant roles, supports location assignments and permission overrides, and scopes caches and mutations by principal and tenant.
- Frontend verification now covers 43 tests across eleven files; lint, type-check, production build, and 16 Playwright smoke tests pass.
- Restricted tenant branding supports optional canonical hex primary/accent colors, allowlisted font tokens, platform-default inheritance, strict schema validation, tenant isolation, optimistic concurrency, and auditable owner/admin updates.
- Tenant branding is isolated from privileged platform HTML/CSS/script/navigation configuration, rejected for the root tenant, scoped in frontend caches by principal and tenant, and applied only to authenticated workspace routes.
- Responsive branding self-service includes read-only access, live preview, publish, conflict recovery, and reset-to-platform behavior without accepting external asset URLs.
- Authenticated tenant logo APIs use separate strict storage, random internal keys, tenant/kind uniqueness, optimistic versions, PNG/JPEG signature and MIME checks, decoded dimension limits, private ETags, owner/admin lifecycle operations, and tenant audit logs.
- Primary and compact tenant logos support authenticated blob loading, scope-bound object URL cleanup, versioned owner/admin upload/delete controls, and tenant-aware workspace header rendering with platform fallback for root and public surfaces.
- Frontend verification now covers 50 tests across fifteen files; lint, type-check, and the production build pass.
- Location branding backend supports strict safe overrides, assigned-location authorization, entitlement-gated owner/admin writes, optimistic concurrency, reset audit, private ETags, and deterministic location-to-tenant-to-platform resolution.
- Location branding frontend resolution is scoped by principal, tenant, and active location; resolved colors/fonts drive authenticated workspace themes while location display names drive visible workspace identity.
- Responsive location-branding self-service supports inheritance previews, assigned-location context, normalized safe overrides, optimistic publish/reset operations, conflict recovery, read-only/root/empty/error states, and race-isolated cache updates.
- Frontend verification now covers 57 tests across eighteen files; lint, type-check, and the production build pass.
- Tenant-scoped restaurant onboarding persists company defaults and the first active location before idempotent owner completion; progress is reconstructed from the database and incomplete owners resume at the correct step.
- Onboarding route enforcement keeps root/member workspaces out, redirects incomplete owners, allows plan and billing recovery routes, and fails closed when setup status cannot be loaded.
- Registration and OAuth workspace provisioning now propagate transaction failures and remove partial users instead of issuing authenticated orphan accounts.
- Known restaurant plan entitlements enforce exact contracts: `max_locations` is numeric and at least one, while `location_branding` is strictly boolean and malformed values fail closed.
- Location branding plan downgrades preserve stored overrides but remove them from effective resolution; ETags now include location, branding, tenant branding, and entitlement state.
- Product settings and location mutations require active staff profiles; location updates additionally require assigned-location access and preserve indistinguishable not-found isolation.
- Authenticated Atlas coverage proves tenant-scoped onboarding, ordinary-plan branding allow/deny/downgrade behavior, restricted/inactive staff denial, and unchanged cross-tenant resources.
- Frontend verification now covers 64 tests across twenty files; lint, type-check, and the production build pass.
- ADR 0007 fixes tenant-owned catalog scope, signed micro-unit quantity storage, exact rational conversion policy, canonical codes, and shared catalog permissions.
- Central quantity arithmetic parses/formats six-decimal fixed-point values and rejects overflow, malformed decimals, invalid factors, and inexact rational results without binary floats.
- Tenant unit catalogs support mass, volume, and count dimensions, canonical unique codes, 0-6 display precision, optimistic updates, soft lifecycle changes, audit events, strict critical schema/index enforcement, and non-leaking tenant scope.
- `catalog.read` and `catalog.manage` extend staff-profile defaults and explicit override enforcement across backend and frontend team management.
- Responsive unit management includes loading, empty, error, denied, read-only, create, edit, deactivate/reactivate, conflict, and tenant-switch-safe states.
- Frontend verification now covers 68 tests across twenty-two files; lint, type-check, and the production build pass. Focused Atlas unit lifecycle, permission, isolation, and schema-rejection tests pass.
- Tenant-wide flat categories support canonical unique codes, active/inactive lifecycle state, optimistic updates, and tenant-safe non-leaking repository scope.
- Category endpoints enforce the shared `catalog.read` and `catalog.manage` permissions, with auditable create, update, deactivate, and reactivate actions.
- Categories use a critical strict MongoDB schema and tenant/code unique index; Go handler, schema-rejection, and model-validation tests cover lifecycle, permissions, isolation, canonicalization, and malformed documents.
- Responsive category management uses tenant-safe UI caching and includes loading, empty, error, denied, read-only, create, edit, lifecycle, and version-conflict states; frontend category validation tests are present.
- Tenant-wide item master data uses immutable lowercase tenant-unique SKUs, required active same-tenant category and unit references on create or reference change, optional descriptions/brands/shelf-life days, EU-14 allergen codes, and a stockable flag.
- Item lifecycle changes use optimistic versions and audit events; existing inactive category/unit references remain valid for unrelated edits, while replacements must target active same-tenant references.
- Item schemas and indexes are strict, and the tenant-scoped API and UI enforce tenant-safe references, lifecycle, caching, and mutation behavior.
- Frontend `package-lock.json` is synchronized and `npm ci` works; TypeScript checking and the production build pass. Focused validation tests passed earlier, while the current Vitest fork-worker timeout remains blocked verification.
- Canonical item conversions are tenant-scoped and unique per item and source unit, exposed through the nested item API.
- Each conversion stores an exact source-to-item-base factor as a positive reduced `int64` numerator and denominator, each at most 1e9; creation requires active same-tenant source and base units with matching dimensions.
- Conversion lifecycle changes use optimistic versions and audit events with strict schemas, critical indexes, and tenant isolation; item base-unit changes are blocked while any conversions exist.
- Standalone responsive conversion management supports item selection, source-unit options, exact factor previews, editing, and lifecycle operations.
- Backend quantity, validation, and handler tests plus the backend build pass; frontend TypeScript checking and the production build pass, while Vitest remains blocked by environment fork-worker timeouts.
- Tenant-wide suppliers support unique supplier codes, contact details, order days, and a default lead time.
- Each supplier and item has one tenant-wide purchasing term with an exact decimal-string microquantity, minor-unit money, MOQ, and optional currency and lead-time overrides.
- Supplier and term creation/reference changes require active same-tenant references; lifecycle operations use optimistic versions and audit events with strict schemas, critical indexes, and tenant isolation.
- Supplier and purchasing-term UI supports responsive supplier management and term management with tenant-safe item selection, exact quantity and money fields, and lifecycle states.
- Backend validation and handler tests plus the backend build pass; frontend TypeScript checking and the production build pass, while Vitest remains blocked by environment fork-worker timeouts.

## v2.0.0 Master-Data And Import Foundation

- Master-data foundation now covers tenant-scoped units, categories, items, canonical conversions, suppliers, and purchasing terms.
- Bounded synchronous master-data CSV imports support UTF-8 comma/semicolon parsing, templates, canonical-header mapping, dry runs, transactional applies, idempotency, tenant-scoped import runs, and bounded error reporting.
- Imports enforce exact limits of 128 KiB per file and 5,000 rows per import; the responsive import wizard provides template selection, mapping, dry-run review, apply, run, and error states.
- `go test ./...` and the Go build pass; `npm ci`, frontend TypeScript checking, and the production build pass. Seventy-three frontend tests passed, but one Vitest fork-worker timeout means the entire run is not green; focused import validation passes.
- Replica-set import integration tests skip when MongoDB is unavailable. Live, Atlas, and Playwright verification are not claimed here.

## Baseline Fixes

- Aligned stale error-message tests with the structured, non-leaking API error policy.
- Fixed a conditional React hook, theme system-change DOM synchronization, custom-page request races, and webhook one-time-secret UI behavior.
- Added frontend test, lint, and build jobs to CI.
- Fixed the Dockerfile to consume a committed non-secret production template and listen on port 8080.
- Installed the Playwright Chromium version matching the locked frontend dependency; all 16 existing browser tests pass.
- Stabilized Atlas-backed suites with package-isolated databases, per-package client reuse, bounded connection retries, and deterministic webhook dispatch synchronization.

## Blocked External Verification

- Authenticated Playwright flows require a running, seeded backend and stable test database.
- Container build verification requires Docker/WSL integration, which is not currently available.
- Vitest verification is currently blocked in this environment because fork workers time out; the frontend package lock is synchronized, `npm ci` works, and TypeScript checking plus the production build pass.

## Active Task

V5 purchasing and goods-receipt workflows are complete. Run replica-set transaction coverage when MongoDB is available.

## Known Follow-Ups

- CI database tests currently depend on the repository's `MONGODB_URI` secret and do not provision a replica set.
- Run the inventory transaction integration tests against a replica set when `MONGODB_URI` is available.
- Add guest actuals, native POS adapters, and reporting after the recipe and sales MVP is stable.
- Resolve unmapped products through a POS mapping, then import the corrected sales data again.
- Run purchase-order and goods-receipt transaction coverage against a MongoDB replica set when `MONGODB_URI` is available.
- Pilot decisions still needed: first customer segment, kitchen/drinks scope, lot-tracking defaults, costing method, approval policy, and available historical data.
- The frontend build warns that the main bundle exceeds 500 kB; address through measured route/vendor splitting rather than arbitrary chunk configuration.
