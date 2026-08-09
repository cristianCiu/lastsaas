# Deployment

## Architecture

The repository deploys one container. A multi-stage Docker build compiles the Go API and React SPA. The final Go process serves API routes and static frontend files on port 8080.

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

## Fly.io

Review and change the placeholder app name and region in `fly.toml`, then configure secrets through Fly.io. Required values are `DATABASE_NAME`, `MONGODB_URI`, `JWT_ACCESS_SECRET`, `JWT_REFRESH_SECRET`, `FRONTEND_URL`, and `APP_NAME`.

Deploy only after explicit approval:

```bash
fly deploy -c fly.toml
```

Do not commit `.env`, populated YAML files, Atlas credentials, OAuth secrets, Stripe keys, or webhook encryption keys.
