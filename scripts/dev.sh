#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env.local}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE. Create it from .env.example and keep it untracked." >&2
  exit 1
fi
if [[ ! -f "$ROOT_DIR/backend/config/dev.yaml" ]]; then
  echo "Missing backend/config/dev.yaml. Copy backend/config/dev.example.yaml locally." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

required=(DATABASE_NAME MONGODB_URI JWT_ACCESS_SECRET JWT_REFRESH_SECRET FRONTEND_URL)
for key in "${required[@]}"; do
  if [[ -z "${!key:-}" ]]; then
    echo "Required variable $key is missing or empty in $ENV_FILE." >&2
    exit 1
  fi
done

mkdir -p "$ROOT_DIR/.pids"

cleanup() {
  jobs -pr | xargs -r kill
  rm -f "$ROOT_DIR/.pids/backend.pid" "$ROOT_DIR/.pids/frontend.pid"
}
trap cleanup EXIT INT TERM

(cd "$ROOT_DIR/backend" && go run ./cmd/server) &
backend_pid=$!
printf '%s\n' "$backend_pid" > "$ROOT_DIR/.pids/backend.pid"

(cd "$ROOT_DIR/frontend" && npm run dev) &
frontend_pid=$!
printf '%s\n' "$frontend_pid" > "$ROOT_DIR/.pids/frontend.pid"

echo "Backend starting at http://localhost:${SERVER_PORT:-4290}"
echo "Frontend starting at http://localhost:${VITE_PORT:-4280}"
echo "Press Ctrl+C to stop both processes."

wait -n "$backend_pid" "$frontend_pid"
