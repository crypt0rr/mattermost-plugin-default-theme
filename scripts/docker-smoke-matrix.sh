#!/usr/bin/env bash

set -euo pipefail

VERSIONS="${MM_VERSIONS:-11.7.0 11.8.5 11.9.1 11.10.1}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"
PLUGIN_BUNDLE="${PLUGIN_BUNDLE:-dist/com.github.crypt0rr.default-theme-0.2.2.tar.gz}"
APP_PORT="${MM_SMOKE_PORT:-18065}"

command -v docker >/dev/null || { echo "docker is required" >&2; exit 1; }
command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
test -f "$PLUGIN_BUNDLE" || { echo "Plugin bundle not found: $PLUGIN_BUNDLE" >&2; exit 1; }

run_version() (
	local version="$1"
	local suffix="${BASHPID}-${version//[^a-zA-Z0-9]/-}"
	local network="mm-default-theme-smoke-${suffix}"
	local db="mm-default-theme-db-${suffix}"
	local app="mm-default-theme-app-${suffix}"
	local base_url="http://127.0.0.1:${APP_PORT}"
	local admin_password="AdminDefaultTheme123!"
	local admin_payload login_payload login_headers token

	cleanup_version() {
		docker rm --force "$app" "$db" >/dev/null 2>&1 || true
		docker network rm "$network" >/dev/null 2>&1 || true
	}

	trap cleanup_version EXIT
	trap 'exit 143' TERM INT
	docker network create "$network" >/dev/null
	docker run --detach --name "$db" --network "$network" \
		--env POSTGRES_USER=mmuser \
		--env POSTGRES_PASSWORD=mostest \
		--env POSTGRES_DB=mattermost \
		"$POSTGRES_IMAGE" >/dev/null

	database_ready=0
	for _ in $(seq 1 120); do
		if docker exec "$db" pg_isready -U mmuser -d mattermost >/dev/null 2>&1; then
			database_ready=1
			break
		fi
		sleep 1
	done
	if [[ "$database_ready" != 1 ]]; then
		echo "PostgreSQL did not become ready for Team Edition ${version}" >&2
		docker inspect "$db" >&2 || true
		docker logs "$db" >&2 || true
		return 1
	fi

	docker run --detach --name "$app" --network "$network" --publish "${APP_PORT}:8065" \
		--env MM_SQLSETTINGS_DRIVERNAME=postgres \
		--env MM_SQLSETTINGS_DATASOURCE="postgres://mmuser:mostest@${db}:5432/mattermost?connect_timeout=10&sslmode=disable" \
		--env MM_SERVICESETTINGS_SITEURL="$base_url" \
		--env MM_PLUGINSETTINGS_ENABLEUPLOADS=true \
		--env MM_PLUGINSETTINGS_REQUIREPLUGINSIGNATURE=false \
		"mattermost/mattermost-team-edition:${version}" >/dev/null

	for _ in $(seq 1 120); do
		if curl --silent --output /dev/null --write-out '%{http_code}' "${base_url}/api/v4/system/ping" | grep -q '^200$'; then
			break
		fi
		sleep 1
	done
	if ! curl --silent --output /dev/null --write-out '%{http_code}' "${base_url}/api/v4/system/ping" | grep -q '^200$'; then
		echo "Mattermost did not become ready for Team Edition ${version}" >&2
		docker inspect "$app" >&2 || true
		docker logs "$app" >&2
		return 1
	fi

	admin_payload="$(jq -n --arg password "$admin_password" \
		'{email:"admin@example.invalid",username:"admin",password:$password,first_name:"Smoke",last_name:"Admin"}')"
	curl --fail-with-body --silent --show-error -H 'Content-Type: application/json' \
		-X POST "${base_url}/api/v4/users" --data "$admin_payload" >/dev/null

	login_payload="$(jq -n --arg password "$admin_password" '{login_id:"admin",password:$password}')"
	login_headers="$(mktemp)"
	trap 'rm -f "$login_headers"; cleanup_version' EXIT
	curl --fail-with-body --silent --show-error -D "$login_headers" -o /dev/null \
		-H 'Content-Type: application/json' -X POST "${base_url}/api/v4/users/login" --data "$login_payload"
	token="$(awk 'tolower($1) == "token:" {gsub("\r", "", $2); print $2}' "$login_headers")"
	rm -f "$login_headers"
	test -n "$token"

	echo "Smoke testing Team Edition ${version}"
	MM_BASE_URL="$base_url" MM_ADMIN_TOKEN="$token" PLUGIN_BUNDLE="$PLUGIN_BUNDLE" \
		bash scripts/smoke-test.sh
)

for version in $VERSIONS; do
	run_version "$version"
done

echo "Team Edition smoke matrix passed"
