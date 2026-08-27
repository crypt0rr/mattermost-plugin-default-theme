# Mattermost Default Theme Plugin

Set the default Mattermost theme for newly created users.

## Behavior

The plugin is server-only. An administrator pastes a custom theme JSON object into the plugin settings in **System Console → Plugins → Default Theme**. The plugin applies that value once, when a human user is created.

Existing users are not changed. Users can select a different theme afterward. Clearing the setting restores Mattermost's normal behavior for future users. Bot accounts are ignored.

The plugin does not enforce a theme, modify existing preferences, expose an API, or store data outside Mattermost's plugin configuration.

## Compatibility

The supported range is Mattermost Team Edition `v11.7.0` through `v11.10.x`. Versions newer than `v11.10.x` may work but are not part of the compatibility claim.

## Configuration

1. Create or export a custom theme from Mattermost user settings.
2. Enable plugin uploads in the server configuration and restart Mattermost:

   ```json
   {
     "PluginSettings": {
       "EnableUploads": true
     }
   }
   ```

3. Install and enable the plugin.
4. Open **System Console → Plugins → Default Theme**.
5. Paste the exported JSON into **Default theme** and save.
6. Create a test account and confirm its theme.

The setting accepts a JSON object whose values are strings. Invalid JSON is rejected by the plugin and the last valid in-memory configuration remains active. Mattermost may normalize or sanitize theme values when it stores the preference.

## Development

Requirements: Go 1.25.13 or a compatible newer Go toolchain. Go 1.25.13 is the pinned release toolchain because it includes security fixes required by the dependency audit.

```sh
make test
make coverage
make security
make check-style
make lint
make bundle
```

The bundle is written to `dist/com.github.crypt0rr.default-theme-0.1.0.tar.gz`.

The security target requires `govulncheck`; CI installs and pins `v1.7.0` automatically.

For local development, enable plugin uploads in the Mattermost server configuration before installing the bundle. The plugin contains binaries for Linux amd64/arm64, Darwin amd64/arm64, and Windows amd64.

### Smoke testing

Run the smoke test against a disposable Team Edition server with a system administrator token:

```sh
MM_BASE_URL=http://localhost:8065 \
MM_ADMIN_TOKEN='<system-admin-token>' \
PLUGIN_BUNDLE=dist/com.github.crypt0rr.default-theme-0.1.0.tar.gz \
bash scripts/smoke-test.sh
```

The test installs and enables the plugin, verifies a configured theme is applied to a new user, verifies an earlier user is unchanged, and verifies the new user can override the theme. Run it against Team Edition `v11.7.0`, the latest `v11.8.x`, the latest `v11.9.x`, and the latest `v11.10.x` images before release.

## License

MIT. See [LICENSE](LICENSE).
