# ADR 0001: Direct Fork and Single-Image Deployment

## Status

Accepted

## Decision

The product extends the existing Go backend directly. Authentication, tenancy, billing, admin APIs, product APIs, and SPA serving remain in one Go binary. The root Dockerfile builds that binary and the React frontend into one image deployed with `fly.toml`.

## Consequences

- No second product backend, Caddy proxy, `Dockerfile.saas`, or `fly.saas.toml` is introduced.
- Product modules live under `backend/internal/` and reuse existing middleware and infrastructure.
- A separate Python process may be added only for forecast computation. It receives narrowly scoped database permissions and cannot mutate inventory or orders.
