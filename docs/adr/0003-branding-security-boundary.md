# ADR 0003: Branding Security Boundary

## Status

Accepted

## Decision

The existing global branding collections and root-admin UI represent platform branding. Their arbitrary HTML, CSS, head markup, custom pages, and analytics capabilities remain root-only.

Restaurant tenant and location branding use separate collections and DTOs. They accept only allowlisted identity fields, color/font tokens, contact/document data, and authorized asset references. Resolution order is location override, tenant branding, then platform defaults.

## Consequences

- Existing platform branding models are not reused as tenant models.
- Tenant assets require tenant/location-scoped authorization and cache keys.
- Public pages before tenant selection always use platform branding.
- Tenant switching clears tenant-derived branding and query state.
