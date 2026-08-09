# Implementation Status

## Current Phase

Phase 1B: restaurant settings and location-scoped storage areas.

## Completed

- Fork checkout configured with `origin` and `upstream` remotes.
- Go 1.25 and locked frontend dependencies installed for local development.
- Backend build and short-mode unit tests pass.
- Frontend unit tests, lint, and production build pass.
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

## Baseline Fixes

- Aligned stale error-message tests with the structured, non-leaking API error policy.
- Fixed a conditional React hook, theme system-change DOM synchronization, custom-page request races, and webhook one-time-secret UI behavior.
- Added frontend test, lint, and build jobs to CI.
- Fixed the Dockerfile to consume a committed non-secret production template and listen on port 8080.
- Installed the Playwright Chromium version matching the locked frontend dependency; all 16 existing browser tests pass.

## Blocked External Verification

- The complete legacy Atlas suite is currently intermittent because the free replica set temporarily reports no primary or times out during connection checkout after repeated connection churn. Location, restaurant settings, storage area, and schema integration suites pass independently when Atlas is available; Stripe also passes independently. This is tracked separately from deterministic code failures.
- Authenticated Playwright flows require a running, seeded backend and stable test database.
- Container build verification requires Docker/WSL integration, which is not currently available.

## Active Task

Implement staff profiles, business roles, and independent location permissions before exposing storage areas or broader operational data to non-admin staff.

## Known Follow-Ups

- CI database tests currently depend on the repository's `MONGODB_URI` secret and do not provision a replica set.
- Pilot decisions still needed: first customer segment, kitchen/drinks scope, lot-tracking defaults, costing method, approval policy, and available historical data.
- The frontend build warns that the main bundle exceeds 500 kB; address through measured route/vendor splitting rather than arbitrary chunk configuration.
