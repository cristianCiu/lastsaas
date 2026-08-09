# ADR 0005: Staff Profile Authorization

## Status

Accepted

## Decision

Core tenant membership roles remain responsible for tenant administration. A non-root membership has one tenant-scoped `staff_profiles` record that independently controls product location access and business permissions.

Missing profiles are inserted at startup without changing existing profiles; existing profiles are validated and malformed records fail startup. New owner, admin, and user memberships default respectively to `company_owner` with all locations, `operations_manager` with all locations, and `viewer` with no locations. Owner and operations-manager profiles receive the initial `storage_areas.read` and `storage_areas.manage` permissions by default; other business roles receive neither. An explicit permission override always wins, including an explicit denial.

Product authorization is evaluated in this order: authentication, tenant membership, billing, active staff profile, assigned location, business permission. Missing or inactive profiles fail closed for location and storage routes. The self-profile endpoint returns inactive profiles with no effective permissions and returns not found for a missing profile. Unknown, cross-tenant, and unassigned locations use the same not-found response and are not disclosed. Core owners and admins manage staff profiles, but admins may manage only core users and the owner profile is protected by the ownership workflow.

## Consequences

- Core admin status does not bypass location or business permission checks on operational routes.
- Restaurant settings retain core membership behavior and do not require a staff profile.
- Profile updates are complete replacements protected by optimistic concurrency and medium-severity audit logs.
- Ownership transfer demotes the expected old owner before promoting the expected target in one transaction, with a unique partial membership index allowing at most one owner per tenant. The new owner profile is reset to active `company_owner` defaults; the former owner profile is reset to active `operations_manager` defaults, including all locations and no overrides.
- Adding a business permission requires updating the strict schema, role defaults, authorization tests, and API documentation.
