# Implementation Sequence

This document refines the two original product plans against the current repository. It preserves their product invariants while reducing security and delivery risk.

## Non-Negotiable Boundaries

- The repository remains one Go modular monolith plus one React application. The future forecast worker is the only planned additional process.
- The authenticated tenant from request context is the authorization source. Request bodies never choose `tenantId`.
- Location access is authorized server-side independently of tenant membership.
- Existing global branding is platform-only. Arbitrary HTML, CSS, head markup, and analytics scripts are never exposed to restaurant tenants.
- Tenant and location branding use restricted, validated tokens and scoped assets.
- Posted inventory movements are append-only and fixed-point. Reversals correct mistakes.
- Forecasts are statistical/ML outputs with uncertainty and a baseline fallback. They never place orders automatically.

## Phase 0: Executable Baseline

1. Keep the fork and upstream remotes explicit.
2. Make backend build/tests and frontend test/lint/build reproducible.
3. Keep frontend dependencies free of known npm advisories.
4. Use the root single-image Dockerfile and `fly.toml`; do not introduce nonexistent `Dockerfile.saas` files.
5. Require a MongoDB replica set for transaction-backed integration tests and development.
6. Record architecture decisions and unresolved external requirements.

Exit requires green non-database gates. Database integration and E2E gates must either pass or name the missing Atlas/credential prerequisite exactly.

## Phase 1A: Product Security Boundary and Locations

1. Add reusable product route registration shared by production and tests.
2. Add tenant-scoped product repository primitives.
3. Implement a validated `locations` model, strict MongoDB schema, critical indexes, and typed accessor.
4. Implement tenant-isolated location list/create APIs with structured errors and owner/admin write authorization.
5. Add numeric `max_locations` enforcement without trusting the client.
6. Add a feature-scoped frontend API and tenant-qualified React Query keys.
7. Deliver the location settings UI and isolation tests.

## Phase 1B: Operational Scope

1. Add active-location selection (location update/deactivation is complete).
2. Add `restaurant_settings` and define `Tenant.Name` as the canonical company name unless superseded by an ADR.
3. Add `staff_profiles` as extensions of memberships, not a second identity system.
4. Add business permissions and location authorization middleware.
5. Add `storage_areas` after location scope is proven.

## Phase 1C: Safe Branding

1. Add restricted tenant branding without arbitrary content.
2. Add a scoped asset pipeline with signature, MIME, dimensions, size, authorization, and lifecycle checks.
3. Add location overrides and deterministic fallback resolution.
4. Split platform branding from authenticated tenant/location branding in the frontend.
5. Add restaurant onboarding after locations, permissions, and branding APIs are stable.

Later phases continue in `WARENWIRTSCHAFT_SAAS_BAUPLAN.md`, preserving the order: master data, inventory journal, recipes/sales, purchasing, forecasting, then pilot hardening.
