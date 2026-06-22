# CLIProxyAPI Plugin

UUAgent manages CLIProxyAPI as a built-in sidecar service from the UUAgent user directory.

For the Windows-first test release, place the real executable and the packaged management panel together:

```text
~/.uuagent/plugins/cliproxyapi/cli-proxy-api.exe
~/.uuagent/plugins/cliproxyapi/management.html
```

On Linux/macOS, the expected executable name changes but the panel filename stays the same:

```text
~/.uuagent/plugins/cliproxyapi/cli-proxy-api
~/.uuagent/plugins/cliproxyapi/management.html
```

UUAgent does not generate or fake these files. If the executable is missing, the Extensions page reports `Missing` and disables Start. Once the executable exists, the Extensions page can start, stop, restart, health-check, and show logs for the service.

UUAgent starts CLIProxyAPI with `MANAGEMENT_STATIC_PATH` pointing at this plugin directory and writes `remote-management.disable-auto-update-panel: true` into the generated CLIProxyAPI config. This keeps the management panel offline-packaged: CLIProxyAPI serves `management.html` from beside the executable instead of downloading it at runtime. If `management.html` is missing, the Extensions page reports the exact path that needs the packaged panel file.

On first start, UUAgent generates local credentials under `~/.uuagent/extensions/cliproxyapi/auth/`:

```text
management.secret
proxy-api.token
```

The Extensions page masks these values, provides copy buttons, and can apply the proxy URL plus `proxy-api.token` to Settings > Models. The proxy token is a local sidecar credential used for UUAgent-to-CLIProxyAPI requests; provider API keys should remain in environment variables.
