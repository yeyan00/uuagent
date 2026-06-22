# CLIProxyAPI Plugin

UUAgent manages CLIProxyAPI as a built-in sidecar service from this directory.

For the Windows-first test release, place the real executable here:

```text
plugins/cliproxyapi/cli-proxy-api.exe
```

On Linux/macOS, the expected executable name is:

```text
plugins/cliproxyapi/cli-proxy-api
```

UUAgent does not generate or fake this binary. If the executable is missing, the Extensions page reports `Missing` and disables Start. Once the executable exists, the Extensions page can start, stop, restart, health-check, and show logs for the service.
