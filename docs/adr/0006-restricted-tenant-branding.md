# ADR 0006: Restricted Tenant Branding Tokens

## Status

Accepted

## Decision

Restaurant tenants use a separate `tenant_branding` collection and `/api/product/branding` contract. The initial contract contains only optional six-digit hex primary/accent colors, an allowlisted font token, optimistic-lock version, and timestamps. Empty tokens inherit platform defaults. Tenant context is authoritative, reads are available to tenant members, writes require the core owner/admin role, root tenants are rejected, and successful writes are audited.

Platform branding remains separate and may contain privileged HTML, CSS, navigation, analytics, and public-page content. None of those fields are accepted by the tenant endpoint. Tenant tokens are applied only to authenticated workspace routes, never public pages or `/last` administration. Uploaded logos and location overrides will use separate scoped models and authorization rather than external asset URLs in this contract.

## Consequences

- Go validation and the strict MongoDB schema enforce the same color and font allowlists.
- A unique tenant index and version filter prevent duplicate records and lost updates.
- Frontend caches include principal and tenant identity and restore platform defaults across scope changes.
- Asset MIME/signature/dimension checks and location fallback remain required before logos or location branding are enabled.
