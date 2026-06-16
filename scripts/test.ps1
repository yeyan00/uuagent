$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

# Keep test output stable in Pi/Git Bash/Windows terminals.
$env:CI = '1'
$env:NO_COLOR = '1'
$env:FORCE_COLOR = '0'
if (-not $env:TERM) { $env:TERM = 'xterm-256color' }

$envFile = Join-Path $Root 'tests/.env'
if (Test-Path $envFile) {
  Get-Content $envFile | ForEach-Object {
    if ($_ -match '^\s*#') { return }
    if ($_ -match '^\s*$') { return }
    if ($_ -match '^(.*?)=(.*)$') {
      [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
  }
}

if (Get-Command go -ErrorAction SilentlyContinue) {
  go test ./...
} else {
  Write-Warning 'go not found; skipping Go tests'
}

if (Get-Command npm -ErrorAction SilentlyContinue) {
  Push-Location web
  npm install
  npm test
  npm run build
  Pop-Location
} else {
  Write-Warning 'npm not found; skipping web build'
}

$legacyPortMatches = Select-String -Path internal/**/*.go,cmd/**/*.go,api/**/*.go,web/src/**/*.tsx,web/vite.config.ts,config.example.yaml,README.md -Pattern '8765' -ErrorAction SilentlyContinue
if ($legacyPortMatches) {
  $legacyPortMatches | ForEach-Object { Write-Error $_ }
  throw 'Found legacy port 8765 references'
}
