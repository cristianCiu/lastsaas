# Deployment

## Architecture

The repository deploys one container image with two Fly process groups. A multi-stage Docker build compiles the Go API, the forecast worker, and the React SPA. The `app` process serves API routes and static frontend files on port 8080. The non-public `worker` process runs `./forecast-worker` and has no HTTP service or inbound listener.

MongoDB Atlas is the only required external runtime dependency. Stripe, Resend, OAuth providers, and DataDog are optional until their features are enabled.

## Local Development

Prerequisites are Go 1.25, Node.js 22, npm, and a MongoDB replica-set URI.

```bash
set -a
source .env
set +a

cd backend
go run ./cmd/server
```

In a second terminal:

```bash
set -a
source .env
set +a

cd frontend
npm run dev
```

The API defaults to `http://localhost:4290`; Vite defaults to `http://localhost:4280` and proxies `/api` to the backend.

## Container Build

```bash
docker build -t lastsaas .
docker run --rm -p 8080:8080 --env-file .env lastsaas
```

The image copies `backend/config/prod.example.yaml`, which contains environment references but no secrets.

The image also contains the `forecast-worker` binary. Fly runs it in the `worker` process group, separately from the HTTP `app` group.

## Fly.io

Review and change the placeholder app name and region in `fly.toml`, then configure secrets through Fly.io. Required values are `DATABASE_NAME`, `MONGODB_URI`, `JWT_ACCESS_SECRET`, `JWT_REFRESH_SECRET`, `FRONTEND_URL`, and `APP_NAME`.

`fly.toml` assigns the HTTP service only to `app`; the `worker` group is therefore not publicly reachable. Keep exactly one worker machine running with:

```bash
fly scale count 1 --group worker
```

The forecast worker handles `SIGTERM` (as well as `SIGINT`) and stops claiming work when its context is cancelled. Fly sends `SIGTERM` during a machine replacement or deploy. If it stops while holding a forecast-job lease, the lease expiry makes that job claimable by the next worker; no second worker or HTTP endpoint is required for recovery. Allow the lease timeout to elapse before treating an interrupted job as permanently failed.

Deploy only after explicit approval:

```bash
fly deploy -c fly.toml
```

Do not commit `.env`, populated YAML files, Atlas credentials, OAuth secrets, Stripe keys, or webhook encryption keys.
