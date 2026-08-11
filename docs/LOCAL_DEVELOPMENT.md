# Local Development

Use native Go and Vite processes for the fastest feedback loop. Use Docker only as a production-image parity check; no application data belongs on the local container filesystem.

## Prerequisites

- Go 1.25+
- Node.js 22+ and npm
- MongoDB Atlas or a local MongoDB replica set with transaction support
- Optional: Docker Desktop with WSL integration for container verification

## One-Time Setup

1. Create an ignored local environment file:

   ```bash
   cp .env.example .env.local
   ```

2. Create independent development values. Never reuse production JWT or encryption keys:

   ```bash
   openssl rand -hex 32
   openssl rand -hex 32
   openssl rand -hex 32
   ```

   Use the first two outputs for the JWT secrets and the third for `WEBHOOK_ENCRYPTION_KEY`. Do not paste secrets into shell history on shared machines; a password manager is preferred.

3. Create the ignored runtime config:

   ```bash
   cp backend/config/dev.example.yaml backend/config/dev.yaml
   ```

4. Use a development-only database such as `lastsaas-local-<developer>`. Never point local development at production or the integration-test database.

5. Install frontend dependencies:

   ```bash
   npm --prefix frontend ci
   ```

## Run

```bash
./scripts/dev.sh
```

- Frontend: `http://localhost:4280`
- Backend: `http://localhost:4290`
- Health: `http://localhost:4290/health`

The Vite server proxies `/api` to the backend. Stop both processes with Ctrl+C.

For a fresh database, initialize the root tenant and owner in a separate terminal. This command is interactive so the root password never enters repository files or shell arguments:

```bash
set -a
source .env.local
set +a
cd backend
go run ./cmd/lastsaas doctor
go run ./cmd/lastsaas setup
```

After setup, open `http://localhost:4280/login`. Run `doctor` again to confirm initialization.

## Google OAuth

Create a separate OAuth client for local development:

- Authorized JavaScript origin: `http://localhost:4280`
- Authorized redirect URI: `http://localhost:4290/api/auth/google/callback`
- `FRONTEND_URL=http://localhost:4280`
- `GOOGLE_REDIRECT_URL=http://localhost:4290/api/auth/google/callback`

Do not share the production OAuth client secret with local environments.

## Production-Parity Container

After enabling Docker Desktop WSL integration:

```bash
docker build -t lastsaas-local .
docker run --rm -p 8080:8080 --env-file .env.local lastsaas-local
```

For this mode, set `FRONTEND_URL=http://localhost:8080`. The image serves both API and SPA on one origin.

## Best Practices

- Commit templates, scripts, and documentation; never commit `.env.local`, `dev.yaml`, credentials, generated tokens, logs, PIDs, or database exports.
- Keep local, test, staging, and production databases separate. `DATABASE_NAME` is an isolation boundary.
- Use least-privilege database users and narrow Atlas network access. Temporary broad access should have an expiry and be removed after setup.
- Give every environment independent JWT and encryption keys. Store production keys in the hosting platform and a recoverable secret vault.
- Keep OAuth clients separate per environment so callback changes cannot break production login.
- Run `git status` before every commit and use secret scanning in CI.
- Validate native development first, then the Docker image, then staging, and only then production.
- Prefer short-lived feature branches for code. Keep `dev/local-environment` for reusable local tooling, not personal secrets or long-running product divergence.
