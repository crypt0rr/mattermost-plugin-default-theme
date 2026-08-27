#!/usr/bin/env bash

set -euo pipefail

: "${MM_BASE_URL:?Set MM_BASE_URL to a disposable Mattermost Team Edition server}"
: "${MM_ADMIN_TOKEN:?Set MM_ADMIN_TOKEN to a system administrator token}"

MM_BASE_URL="${MM_BASE_URL%/}"
PLUGIN_BUNDLE="${PLUGIN_BUNDLE:-dist/com.github.crypt0rr.default-theme-0.1.0.tar.gz}"
THEME_JSON="${THEME_JSON:-{\"sidebarBg\":\"#145DBF\",\"sidebarText\":\"#FFFFFF\"}}"
PLUGIN_ID="com.github.crypt0rr.default-theme"

command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
test -f "$PLUGIN_BUNDLE" || { echo "Plugin bundle not found: $PLUGIN_BUNDLE" >&2; exit 1; }

api() {
	curl --fail-with-body --silent --show-error \
		-H "Authorization: Bearer ${MM_ADMIN_TOKEN}" \
		-H "Content-Type: application/json" \
		"$@"
}

echo "Installing ${PLUGIN_BUNDLE}"
curl --fail-with-body --silent --show-error \
	-H "Authorization: Bearer ${MM_ADMIN_TOKEN}" \
	-F "plugin=@${PLUGIN_BUNDLE}" \
	-F "force=true" \
	"${MM_BASE_URL}/api/v4/plugins" >/dev/null

api -X POST "${MM_BASE_URL}/api/v4/plugins/${PLUGIN_ID}/enable" >/dev/null

patch_theme() {
	local theme="$1"
	local payload
	payload="$(jq -n --arg theme "$theme" \
		'{PluginSettings:{Plugins:{"com.github.crypt0rr.default-theme":{DefaultTheme:$theme}}}}')"
	api -X PUT "${MM_BASE_URL}/api/v4/config/patch" --data "$payload" >/dev/null
}

create_user() {
	local label="$1"
	local suffix="$(date +%s%N)-${RANDOM}"
	local username="dt-${label}-${suffix: -12}"
	local payload
	payload="$(jq -n \
		--arg username "$username" \
		--arg email "${username}@example.invalid" \
		'{username:$username,email:$email,password:"DefaultTheme123!",first_name:"Default",last_name:"Theme"}')"
	api -X POST "${MM_BASE_URL}/api/v4/users" --data "$payload" | jq -er '.id'
}

theme_for_user() {
	local user_id="$1"
	api "${MM_BASE_URL}/api/v4/users/${user_id}/preferences" | jq -cS '
		[.[] | select(.category == "theme" and .name == "")] |
		if length == 0 then null else (.[0].value | fromjson) end
	'
}

wait_for_theme() {
	local user_id="$1"
	local expected="$2"
	local actual=""
	for _ in $(seq 1 30); do
		actual="$(theme_for_user "$user_id")"
		if [[ "$actual" == "$expected" ]]; then
			return 0
		fi
		sleep 1
	done
	echo "Expected theme ${expected} for user ${user_id}, got ${actual}" >&2
	return 1
}

expected_theme="$(jq -cS . <<<"${THEME_JSON}")"
override_theme='{"sidebarBg":"#000000"}'
expected_override="$(jq -cS . <<<"${override_theme}")"

patch_theme ""
before_user="$(create_user before)"
if [[ "$(theme_for_user "$before_user")" != "null" ]]; then
	echo "A user created before configuration unexpectedly received a theme" >&2
	exit 1
fi

patch_theme "$THEME_JSON"
after_user="$(create_user after)"
wait_for_theme "$after_user" "$expected_theme"

if [[ "$(theme_for_user "$before_user")" != "null" ]]; then
	echo "An existing user was modified when the theme was configured" >&2
	exit 1
fi

override_payload="$(jq -n --arg user_id "$after_user" --arg value "$override_theme" \
	'[{user_id:$user_id,category:"theme",name:"",value:$value}]')"
api -X PUT "${MM_BASE_URL}/api/v4/users/${after_user}/preferences" --data "$override_payload" >/dev/null
wait_for_theme "$after_user" "$expected_override"

echo "Smoke test passed for ${MM_BASE_URL}"
