param(
  [switch]$SkipTests,
  [string]$CLIProxyAPIRoot = "$env:USERPROFILE\.uuagent\plugins\cliproxyapi",
  [string]$OutputPath = "dist\uuagent.exe"
)

$ErrorActionPreference = 'Stop'

$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$BinaryName = 'cli-proxy-api.exe'
$SourceBinary = Join-Path $CLIProxyAPIRoot $BinaryName
$SourcePanel = Join-Path $CLIProxyAPIRoot 'management.html'
$EmbedRoot = Join-Path $Root 'internal\config\embedded_plugins\cliproxyapi'
$EmbedBinary = Join-Path $EmbedRoot $BinaryName
$EmbedPanel = Join-Path $EmbedRoot 'management.html'

function Assert-File($Path, $Name) {
  if (-not (Test-Path -LiteralPath $Path)) {
    throw "$Name missing: $Path"
  }
  $Item = Get-Item -LiteralPath $Path
  if ($Item.Length -le 0) {
    throw "$Name is empty: $Path"
  }
  return $Item
}

function Invoke-Step($Name, [scriptblock]$Action) {
  Write-Host "==> $Name"
  & $Action
}

Invoke-Step 'Validate CLIProxyAPI assets' {
  $Binary = Assert-File $SourceBinary 'CLIProxyAPI binary'
  $Panel = Assert-File $SourcePanel 'CLIProxyAPI management panel'
  if ($Binary.Length -lt 1MB) {
    throw "CLIProxyAPI binary is too small, refusing to embed likely placeholder: $SourceBinary ($($Binary.Length) bytes)"
  }
  if ($Panel.Length -lt 100KB) {
    throw "CLIProxyAPI management panel is too small, refusing to embed likely placeholder: $SourcePanel ($($Panel.Length) bytes)"
  }
  Write-Host "    binary: $($Binary.Length) bytes"
  Write-Host "    panel:  $($Panel.Length) bytes"
}

Invoke-Step 'Copy CLIProxyAPI assets into embed input' {
  New-Item -ItemType Directory -Path $EmbedRoot -Force | Out-Null
  Copy-Item -LiteralPath $SourceBinary -Destination $EmbedBinary -Force
  Copy-Item -LiteralPath $SourcePanel -Destination $EmbedPanel -Force
}

if (-not $SkipTests) {
  Invoke-Step 'Run Go tests' {
    go test ./...
  }
}

Invoke-Step 'Build Web UI' {
  Push-Location 'web'
  try {
    npm install
    if (-not $SkipTests) {
      npm test
    }
    npm run build
  } finally {
    Pop-Location
  }
}

Invoke-Step 'Build UUAgent executable' {
  $OutputDir = Split-Path -Parent $OutputPath
  if ($OutputDir) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
  }
  go build -o $OutputPath ./cmd/uuagent
  $Exe = Assert-File (Join-Path $Root $OutputPath) 'UUAgent executable'
  Write-Host "    exe: $($Exe.FullName) ($($Exe.Length) bytes)"
}

Invoke-Step 'Smoke setup and embedded plugin release' {
  $SmokeHome = Join-Path $env:TEMP ("uuagent-package-smoke-" + $PID)
  $PreviousHome = $env:UUAGENT_HOME
  $env:UUAGENT_HOME = $SmokeHome
  try {
    & (Join-Path $Root $OutputPath) --setup
    if ($LASTEXITCODE -ne 0) {
      throw "uuagent --setup failed with exit code $LASTEXITCODE"
    }
    $ReleasedBinary = Join-Path $SmokeHome "plugins\cliproxyapi\$BinaryName"
    $ReleasedPanel = Join-Path $SmokeHome 'plugins\cliproxyapi\management.html'
    $Binary = Assert-File $ReleasedBinary 'released CLIProxyAPI binary'
    $Panel = Assert-File $ReleasedPanel 'released CLIProxyAPI management panel'
    if ($Binary.Length -lt 1MB) {
      throw "released CLIProxyAPI binary is too small: $($Binary.Length) bytes"
    }
    if ($Panel.Length -lt 100KB) {
      throw "released CLIProxyAPI management panel is too small: $($Panel.Length) bytes"
    }
    Write-Host "    released binary: $($Binary.Length) bytes"
    Write-Host "    released panel:  $($Panel.Length) bytes"
  } finally {
    $env:UUAGENT_HOME = $PreviousHome
  }
}

Write-Host "Package ready: $((Resolve-Path -LiteralPath $OutputPath).Path)"
