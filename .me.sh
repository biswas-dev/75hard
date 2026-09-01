#!/usr/bin/env bash
# 75hard adapter for the shared ./scripts/server runner (github.com/anchoo2kewl/me).
#
# Usage:  ./.me/scripts/server local start 75hard
#         ./.me/scripts/server prod status 75hard

PROJECT_NAME="75hard"
PROJECT_DOMAIN="75hard.biswas.me"
PROJECT_REPO="anchoo2kewl/75hard"
PROJECT_STACK="Go + React + SQLite"
PROJECT_PORT_BACKEND=8087
PROJECT_PORT_FRONTEND=5175
PROJECT_DB="sqlite"

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
API_DIR="$PROJECT_ROOT/api"
WEB_DIR="$PROJECT_ROOT/web"
DATA_DIR="$PROJECT_ROOT/data"

_deploy_path() { echo "/opt/75hard"; }
_container()   { echo "75hard-75hard-1"; }

# ---- local ----

local_start() {
    print_header "Starting 75hard"
    mkdir -p "$DATA_DIR/photos"

    if port_alive "$PROJECT_PORT_BACKEND"; then
        print_warning "Port $PROJECT_PORT_BACKEND already in use; stopping first"
        kill_port "$PROJECT_PORT_BACKEND"
    fi

    print_status "Building web"
    (cd "$WEB_DIR" && npm run build) || { print_error "web build failed"; return 1; }

    print_status "Starting API on :$PROJECT_PORT_BACKEND (serving the built SPA)"
    (
        cd "$API_DIR" || exit 1
        ENV=development \
        PORT="$PROJECT_PORT_BACKEND" \
        DB_PATH="$DATA_DIR/75hard.db" \
        PHOTOS_DIR="$DATA_DIR/photos" \
        FRONTEND_DIST="$WEB_DIR/dist" \
        JWT_SECRET="${JWT_SECRET:-dev-secret-change-me}" \
        go run ./cmd/api
    ) > "$PROJECT_ROOT/api.log" 2>&1 &

    sleep 3
    port_status "$PROJECT_PORT_BACKEND" "API"
    print_success "http://localhost:$PROJECT_PORT_BACKEND"
}

# dev runs the API and Vite separately, so the frontend hot-reloads.
local_dev() {
    print_header "75hard dev"
    mkdir -p "$DATA_DIR/photos"

    print_status "API on :$PROJECT_PORT_BACKEND"
    (
        cd "$API_DIR" || exit 1
        ENV=development \
        PORT="$PROJECT_PORT_BACKEND" \
        DB_PATH="$DATA_DIR/75hard.db" \
        PHOTOS_DIR="$DATA_DIR/photos" \
        JWT_SECRET="${JWT_SECRET:-dev-secret-change-me}" \
        go run ./cmd/api
    ) > "$PROJECT_ROOT/api.log" 2>&1 &

    sleep 2
    print_status "Vite on :$PROJECT_PORT_FRONTEND (proxying /api to the local API)"
    (cd "$WEB_DIR" && npm run dev)
}

local_stop() {
    print_header "Stopping 75hard"
    kill_port "$PROJECT_PORT_BACKEND"
    kill_port "$PROJECT_PORT_FRONTEND"
    print_success "Stopped"
}

local_restart() { local_stop; sleep 1; local_start; }

local_status() {
    print_header "75hard status"
    port_status "$PROJECT_PORT_BACKEND" "API"
    port_status "$PROJECT_PORT_FRONTEND" "Vite"
    if [ -f "$DATA_DIR/75hard.db" ]; then
        print_status "Database: $DATA_DIR/75hard.db ($(du -h "$DATA_DIR/75hard.db" | cut -f1))"
    else
        print_warning "No local database yet"
    fi
}

local_logs() { tail -f "$PROJECT_ROOT/api.log"; }

local_test() { local_test_backend && local_test_frontend; }

local_test_backend() {
    print_header "Go tests"
    (cd "$API_DIR" && go vet ./... && go test ./...)
}

local_test_frontend() {
    print_header "Frontend typecheck and build"
    (cd "$WEB_DIR" && npx tsc --noEmit && npx vite build)
}

# Migrations run automatically at boot; this just starts the server briefly.
local_db_migrate() {
    print_header "Applying migrations"
    (
        cd "$API_DIR" || exit 1
        DB_PATH="$DATA_DIR/75hard.db" PHOTOS_DIR="$DATA_DIR/photos" \
        timeout 5 go run ./cmd/api 2>&1 | grep -i migration || true
    )
    print_success "Done"
}

local_db_reset() {
    print_header "Resetting the local database"
    print_warning "This deletes $DATA_DIR/75hard.db and every local photo."
    read -r -p "Type 'reset' to confirm: " reply
    [ "$reply" = "reset" ] || { print_status "Cancelled"; return 0; }
    rm -f "$DATA_DIR"/75hard.db*
    rm -rf "${DATA_DIR:?}/photos"
    mkdir -p "$DATA_DIR/photos"
    print_success "Reset"
}

local_users() {
    require_cmd sqlite3 || return 1
    sqlite3 -header -column "$DATA_DIR/75hard.db" \
        "SELECT id, email, name, timezone, is_admin, created_at FROM users WHERE deleted_at IS NULL;"
}

# ---- remote ----

remote_status() {
    local server; server="$(resolve_server "$1")"
    ssh_cmd "$server" "cd $(_deploy_path) && sudo docker compose ps"
}

remote_logs() {
    local server; server="$(resolve_server "$1")"
    ssh_cmd "$server" "cd $(_deploy_path) && sudo docker compose logs --tail=200 -f"
}

remote_health() {
    local server; server="$(resolve_server "$1")"
    ssh_cmd "$server" "curl -fsS http://127.0.0.1:13431/health && echo"
}

remote_restart() {
    local server; server="$(resolve_server "$1")"
    ssh_cmd "$server" "cd $(_deploy_path) && sudo docker compose restart"
}

remote_users() {
    local server; server="$(resolve_server "$1")"
    # sqlite3 isn't in the runtime image, so read the DB from the host bind mount.
    ssh_cmd "$server" "sudo sqlite3 -header -column $(_deploy_path)/data/75hard.db \
        'SELECT id, email, name, is_admin, created_at FROM users WHERE deleted_at IS NULL;'"
}
