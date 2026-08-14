# LastSaaS Development Rules

## Validation

LastSaaS uses hybrid validation: Go-side (`validate` struct tags via go-playground/validator) and MongoDB JSON Schema (`internal/db/schema.go`).

**When modifying model structs in `internal/models/`:**
1. Update `validate` struct tags on the model
2. Update the corresponding MongoDB JSON Schema in `internal/db/schema.go`
3. Keep both in sync — the Go tags and MongoDB schema must enforce the same constraints
4. Run `cd backend && go test ./internal/validation/...` to verify

**When adding a new collection that accepts user/API writes:**
1. Add `validate` tags to the model struct
2. Add a schema function to `internal/db/schema.go` and include it in `AllSchemas()`
3. Add tests in `internal/validation/validate_test.go`

## System Logging

Use `syslog.Logger` for all significant system events. Severity levels: critical, high, medium, low, debug.

## Build Verification

Always verify after changes:
```bash
cd backend && go build ./...
cd frontend && npx tsc --noEmit
```

## Release Documentation

For every version update, update `README.md` in the same commit with the version number, release details, and verification status. Keep `VERSIONS.md` and `VERSION` consistent with the README.

## Deployment Architecture (CRITICAL)

This repository is a direct fork, not a product that depends on a second LastSaaS backend. Extend the existing Go modular monolith and do not create a parallel authentication, tenant, billing, or admin service.

The root `Dockerfile` builds the Go API and React application into one image. The Go binary serves both `/api/*` and the frontend SPA. Deployment uses:

```bash
fly deploy -c fly.toml
```

See `deploy.md` before changing or running deployment commands. Keep secrets in the deployment environment; never bake a populated config or `.env` file into the image.
