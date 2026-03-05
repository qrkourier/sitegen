#!/usr/bin/env bash
set -euo pipefail

# Convenience wrapper for docker compose operations.
# Sources .env.sh (if present) to export secrets, then delegates to
# docker compose with the file list from .env's COMPOSE_FILE.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Export secrets / extra env vars from .env.sh if it exists.
if [[ -f .env.sh ]]; then
    # shellcheck disable=SC1091
    source .env.sh
fi

usage() {
    cat <<'EOF'
Usage: ./run.sh <command> [options]

Commands:
  up        Build (if needed) and start in the background (force-recreate)
  down      Stop and remove containers (preserves volumes)
  restart   Shorthand for down + up
  logs      Tail container logs (pass -f to follow)
  ps        Show container status
  build     Build images without starting
  rebuild   Force-build images then recreate containers
  config    Print the resolved compose configuration

Write mode:
  Set WRITE_MODE=true and CONTENT_VOLUME_MODE=rw in .env to enable the
  markdown editor. The sitegen container reads WRITE_MODE from the
  environment and mounts content as read-write when CONTENT_VOLUME_MODE=rw.

Options are forwarded to docker compose where applicable.
EOF
}

compose() {
    docker compose "$@"
}

case "${1:-}" in
    up)
        shift
        compose up --detach --force-recreate --remove-orphans "$@"
        ;;
    down)
        shift
        compose down --remove-orphans "$@"
        ;;
    restart)
        shift
        compose down --remove-orphans "$@"
        compose up --detach --force-recreate --remove-orphans "$@"
        ;;
    logs)
        shift
        compose logs "$@"
        ;;
    ps)
        shift
        compose ps "$@"
        ;;
    build)
        shift
        compose build "$@"
        ;;
    rebuild)
        shift
        compose build --no-cache "$@"
        compose up --detach --force-recreate --remove-orphans "$@"
        ;;
    config)
        shift
        compose config "$@"
        ;;
    ""|help|-h|--help)
        usage
        ;;
    *)
        echo "Unknown command: $1" >&2
        usage
        exit 1
        ;;
esac
