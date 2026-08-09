# ADR 0002: Tenant and Location Authorization

## Status

Accepted

## Decision

Product routes derive the tenant from authenticated middleware context. Product repositories require a tenant ID for every operation. Location-scoped operations additionally require a location authorized through the user's staff profile.

Core membership roles continue to control tenant administration. Restaurant business roles and location assignments extend a membership through `staff_profiles`; they do not replace identity or membership.

## Consequences

- Client-provided tenant IDs are selectors, never authorization proof.
- Product query caches include tenant ID and, when applicable, location ID.
- Cross-tenant resources normally return not found to avoid disclosing existence.
- The first vertical slice is tenant-isolated location list/create, before branding or inventory.
