param(
  [string]$Version = "",
  [string]$OutputDir = "dist",
  [string[]]$Targets = @(
    "windows/amd64",
    "windows/arm64",
    "linux/amd64",
    "linux/arm64",
    "darwin/amd64",
    "darwin/arm64"
  ),
  [switch]$NoClean
)

$ErrorActionPreference = "Stop"

function Find-Go {
  $cmd = Get-Command go -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $bundled = Join-Path $env:USERPROFILE ".codex\tools\go1.26.3\go\bin\go.exe"
  if (Test-Path $bundled) { return $bundled }
  throw "Go was not found in PATH or $bundled"
}

function Git-Value {
  param([string[]]$GitArgs, [string]$Fallback)
  try {
    $output = & git @GitArgs 2>$null
    if ($LASTEXITCODE -eq 0) {
      $value = ($output | Select-Object -First 1).Trim()
      if ($value) { return $value }
    }
  } catch {
  }
  return $Fallback
}

function Write-Utf8NoBom {
  param([string]$Path, [string]$Text)
  $encoding = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, $Text, $encoding)
}

function Copy-Tree {
  param([string]$From, [string]$To)
  if (!(Test-Path $From)) { return }
  New-Item -ItemType Directory -Force -Path $To | Out-Null
  Copy-Item -Path (Join-Path $From "*") -Destination $To -Recurse -Force
}

function Compress-ZipWithRetry {
  param([string]$Path, [string]$DestinationPath)
  for ($i = 1; $i -le 5; $i++) {
    try {
      Compress-Archive -Path $Path -DestinationPath $DestinationPath -Force
      return
    } catch {
      if ($i -eq 5) { throw }
      Start-Sleep -Seconds $i
    }
  }
}

function New-StartScripts {
  param([string]$Stage, [string]$OS, [string]$Binary)

  if ($OS -eq "windows") {
    Write-Utf8NoBom (Join-Path $Stage "start-hub.ps1") @"
`$ErrorActionPreference = "Stop"
`$Root = `$PSScriptRoot
& (Join-Path `$Root "$Binary") hub -c (Join-Path `$Root "configs\dev\hub.json")
"@
    Write-Utf8NoBom (Join-Path $Stage "start-agent.ps1") @"
param([string]`$Config = "configs\dev\laptop.json")
`$ErrorActionPreference = "Stop"
`$Root = `$PSScriptRoot
& (Join-Path `$Root "$Binary") agent -c (Join-Path `$Root `$Config)
"@
    Write-Utf8NoBom (Join-Path $Stage "start-gui.ps1") @"
param([string]`$Config = "configs\dev\laptop.json")
`$ErrorActionPreference = "Stop"
`$Root = `$PSScriptRoot
& (Join-Path `$Root "$Binary") gui -c (Join-Path `$Root `$Config)
"@
    Write-Utf8NoBom (Join-Path $Stage "start-app.cmd") @"
@echo off
setlocal
set "ROOT=%~dp0"
if "%~1"=="" (
  set "CONFIG=%ROOT%configs\dev\laptop.json"
) else (
  set "CONFIG=%~1"
)
"%ROOT%$Binary" app -c "%CONFIG%"
"@
    Write-Utf8NoBom (Join-Path $Stage "start-hub-app.cmd") @"
@echo off
setlocal
set "ROOT=%~dp0"
"%ROOT%$Binary" app -c "%ROOT%configs\dev\server.json" -hub-config "%ROOT%configs\dev\hub.json"
"@
    Write-Utf8NoBom (Join-Path $Stage "start-agent.cmd") @"
@echo off
setlocal
set "ROOT=%~dp0"
if "%~1"=="" (
  set "CONFIG=%ROOT%configs\dev\laptop.json"
) else (
  set "CONFIG=%~1"
)
"%ROOT%$Binary" agent -c "%CONFIG%"
"@
    Write-Utf8NoBom (Join-Path $Stage "start-gui.cmd") @"
@echo off
setlocal
set "ROOT=%~dp0"
if "%~1"=="" (
  set "CONFIG=%ROOT%configs\dev\laptop.json"
) else (
  set "CONFIG=%~1"
)
"%ROOT%$Binary" gui -c "%CONFIG%"
"@
    return
  }

  Write-Utf8NoBom (Join-Path $Stage "start-hub.sh") @"
#!/usr/bin/env sh
set -eu
DIR=`$(CDPATH= cd -- "`$(dirname -- "`$0")" && pwd)
exec "`$DIR/$Binary" hub -c "`$DIR/configs/dev/hub.json"
"@
  Write-Utf8NoBom (Join-Path $Stage "start-agent.sh") @"
#!/usr/bin/env sh
set -eu
DIR=`$(CDPATH= cd -- "`$(dirname -- "`$0")" && pwd)
CONFIG=`${1:-configs/dev/laptop.json}
exec "`$DIR/$Binary" agent -c "`$DIR/`$CONFIG"
"@
  Write-Utf8NoBom (Join-Path $Stage "start-gui.sh") @"
#!/usr/bin/env sh
set -eu
DIR=`$(CDPATH= cd -- "`$(dirname -- "`$0")" && pwd)
CONFIG=`${1:-configs/dev/laptop.json}
exec "`$DIR/$Binary" gui -c "`$DIR/`$CONFIG"
"@
  Write-Utf8NoBom (Join-Path $Stage "start-app.sh") @"
#!/usr/bin/env sh
set -eu
DIR=`$(CDPATH= cd -- "`$(dirname -- "`$0")" && pwd)
CONFIG=`${1:-configs/dev/laptop.json}
exec "`$DIR/$Binary" app -c "`$DIR/`$CONFIG"
"@
  Write-Utf8NoBom (Join-Path $Stage "start-hub-app.sh") @"
#!/usr/bin/env sh
set -eu
DIR=`$(CDPATH= cd -- "`$(dirname -- "`$0")" && pwd)
exec "`$DIR/$Binary" app -c "`$DIR/configs/dev/server.json" -hub-config "`$DIR/configs/dev/hub.json"
"@
}

function New-QuickStart {
  param([string]$Stage, [string]$OS, [string]$Binary)

  $runPrefix = if ($OS -eq "windows") { ".\$Binary" } else { "./$Binary" }
  $lines = @(
    "# QuickDrop Portable Package",
    "",
    "This package is a portable build. It does not install a service or modify the system.",
    "",
    "Double-click QuickDropApp.exe on Windows, or run $runPrefix with no arguments, to open the integrated app mode.",
    "App mode starts the GUI and Agent together. Closing the GUI page or pressing Close app shuts down the background services.",
    "",
    "## First local demo",
    "",
    "Terminal 1:",
    "",
    '```',
    "$runPrefix init-dev",
    "$runPrefix hub -c configs/dev/hub.json",
    '```',
    "",
    "Terminal 2:",
    "",
    '```',
    "$runPrefix agent -c configs/dev/laptop.json",
    '```',
    "",
    "Terminal 3:",
    "",
    '```',
    "$runPrefix agent -c configs/dev/workstation.json",
    '```',
    "",
    "Terminal 4:",
    "",
    '```',
    "$runPrefix gui -c configs/dev/laptop.json",
    '```',
    "",
    "Open http://127.0.0.1:47900.",
    "",
    "## Real devices",
    "",
    "Use one device as Hub. Each other device runs Agent and GUI with its own config.",
    "The GUI Settings panel can edit device identity, language, Hub URL, SSH tunnel, and directories.",
    "",
    "For SSH tunnel mode, keep the Hub listening on 127.0.0.1 and point each Agent/GUI at the local forwarded port."
  )
  $text = $lines -join [Environment]::NewLine
  if ($OS -ne "windows") {
    $text += [Environment]::NewLine + ( @(
      'If your archive tool does not preserve executable bits:',
      '',
      '```',
      'chmod +x quickdrop start-*.sh',
      '```'
    ) -join [Environment]::NewLine )
  }
  Write-Utf8NoBom (Join-Path $Stage "QUICKSTART.md") $text
}

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$mainPackage = Join-Path (Join-Path $root "cmd") "quickdrop"
$go = Find-Go
if (!$Version) {
  if ($env:GITHUB_REF_NAME) {
    $Version = $env:GITHUB_REF_NAME
  } else {
    $Version = Git-Value -GitArgs @("describe", "--tags", "--always", "--dirty") -Fallback "dev"
  }
}
$Commit = Git-Value -GitArgs @("rev-parse", "--short", "HEAD") -Fallback "unknown"
$BuiltAt = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

$dist = Join-Path $root $OutputDir
if (!$NoClean -and (Test-Path $dist)) {
  $resolvedRoot = (Resolve-Path $root).Path
  $resolvedDist = (Resolve-Path $dist).Path
  if (!$resolvedDist.StartsWith($resolvedRoot)) {
    throw "Refusing to remove output directory outside repo: $resolvedDist"
  }
  Remove-Item -LiteralPath $dist -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $dist | Out-Null

$checksums = @()
$oldGOOS = $env:GOOS
$oldGOARCH = $env:GOARCH
$oldCGO = $env:CGO_ENABLED
try {
  foreach ($target in $Targets) {
    $parts = $target.Split("/")
    if ($parts.Count -ne 2) { throw "Invalid target: $target" }
    $os = $parts[0]
    $arch = $parts[1]
    $pkgName = "quickdrop-$Version-$os-$arch"
    $stage = Join-Path $dist $pkgName
    New-Item -ItemType Directory -Force -Path $stage | Out-Null

    $binary = if ($os -eq "windows") { "quickdrop.exe" } else { "quickdrop" }
    $binaryPath = Join-Path $stage $binary
    $env:GOOS = $os
    $env:GOARCH = $arch
    $env:CGO_ENABLED = "0"
    $ldflags = "-s -w -X main.version=$Version -X main.commit=$Commit -X main.builtAt=$BuiltAt"
    Write-Host "Building $target -> $pkgName"
    & $go build -trimpath "-ldflags=$ldflags" -o $binaryPath $mainPackage
    if ($LASTEXITCODE -ne 0) { throw "go build failed for $target" }
    if ($os -eq "windows") {
      $guiBinaryPath = Join-Path $stage "QuickDropApp.exe"
      $guiLdflags = "$ldflags -H=windowsgui"
      & $go build -trimpath "-ldflags=$guiLdflags" -o $guiBinaryPath $mainPackage
      if ($LASTEXITCODE -ne 0) { throw "go build GUI launcher failed for $target" }
    }

    Copy-Item -LiteralPath (Join-Path $root "README.md") -Destination $stage -Force
    Copy-Item -LiteralPath (Join-Path $root "AGENTS.md") -Destination $stage -Force
    Copy-Tree (Join-Path $root "configs") (Join-Path $stage "configs")
    Copy-Item -LiteralPath (Join-Path (Join-Path $root "tests") "INTEGRATION.md") -Destination $stage -Force
    New-StartScripts $stage $os $binary
    New-QuickStart $stage $os $binary

    if ($os -eq "windows") {
      $archive = Join-Path $dist "$pkgName.zip"
      Compress-ZipWithRetry -Path (Join-Path $stage "*") -DestinationPath $archive
    } else {
      $archive = Join-Path $dist "$pkgName.tar.gz"
      & tar -czf $archive -C $dist $pkgName
      if ($LASTEXITCODE -ne 0) { throw "tar failed for $pkgName" }
    }
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
    $checksums += "$hash  $(Split-Path -Leaf $archive)"
  }
} finally {
  $env:GOOS = $oldGOOS
  $env:GOARCH = $oldGOARCH
  $env:CGO_ENABLED = $oldCGO
}

Write-Utf8NoBom (Join-Path $dist "checksums.txt") (($checksums -join [Environment]::NewLine) + [Environment]::NewLine)
Write-Host "Packages written to $dist"
