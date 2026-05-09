param(
  [Parameter(Mandatory = $true)]
  [string]$HostName,
  [int]$LocalPort = 48991,
  [int]$RemotePort = 47891,
  [string]$RemoteDir = "quickdrop-remote-smoke",
  [switch]$KeepRemote
)

$ErrorActionPreference = "Stop"

function Find-Go {
  $cmd = Get-Command go -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $bundled = Join-Path $env:USERPROFILE ".codex\tools\go1.26.3\go\bin\go.exe"
  if (Test-Path $bundled) { return $bundled }
  throw "Go was not found in PATH or $bundled"
}

function Invoke-SSH {
  param([string]$Command)
  $output = & ssh -o BatchMode=yes -o ConnectTimeout=8 $HostName $Command 2>&1
  if ($LASTEXITCODE -ne 0) {
    throw "ssh $HostName failed for command [$Command]: $output"
  }
  return $output
}

function Write-Json {
  param([string]$Path, [object]$Value)
  $dir = Split-Path -Parent $Path
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $json = $Value | ConvertTo-Json -Depth 20
  $encoding = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, $json + [Environment]::NewLine, $encoding)
}

function Wait-Health {
  param([string]$URL)
  $deadline = (Get-Date).AddSeconds(20)
  do {
    try {
      $health = Invoke-RestMethod "$URL/api/health" -TimeoutSec 2
      if ($health.ok) { return }
    } catch {
      Start-Sleep -Milliseconds 300
    }
  } while ((Get-Date) -lt $deadline)
  throw "Timed out waiting for $URL/api/health"
}

function Wait-Log {
  param([string]$Path, [string]$Needle)
  $deadline = (Get-Date).AddSeconds(20)
  do {
    if ((Test-Path $Path) -and ((Get-Content $Path -Raw) -like "*$Needle*")) { return }
    Start-Sleep -Milliseconds 300
  } while ((Get-Date) -lt $deadline)
  throw "Timed out waiting for [$Needle] in $Path"
}

$go = Find-Go
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$work = Join-Path $root "data\remote-smoke-$($HostName -replace '[^A-Za-z0-9_.-]', '_')"
$binDir = Join-Path $work "bin"
$cfgDir = Join-Path $work "configs"
$logDir = Join-Path $work "logs"
New-Item -ItemType Directory -Force -Path $binDir, $cfgDir, $logDir | Out-Null

$uname = (Invoke-SSH "uname -s" | Select-Object -First 1).Trim()
$machine = (Invoke-SSH "uname -m" | Select-Object -First 1).Trim()
if ($uname -like "MINGW*" -or $uname -like "MSYS*" -or $uname -like "CYGWIN*") {
  $remoteGOOS = "windows"
  $remoteGOARCH = "amd64"
  $remoteBinary = "quickdrop.exe"
} elseif ($uname -eq "Darwin") {
  $remoteGOOS = "darwin"
  $remoteGOARCH = if ($machine -match "arm64|aarch64") { "arm64" } else { "amd64" }
  $remoteBinary = "quickdrop"
} elseif ($uname -eq "Linux") {
  $remoteGOOS = "linux"
  $remoteGOARCH = if ($machine -match "arm64|aarch64") { "arm64" } else { "amd64" }
  $remoteBinary = "quickdrop"
} else {
  throw "Unsupported remote uname: $uname $machine"
}

Write-Host "Remote $HostName reports $uname $machine; building $remoteGOOS/$remoteGOARCH"

$localExe = Join-Path $binDir "quickdrop-local.exe"
& $go build -o $localExe "$root\cmd\quickdrop"
if ($LASTEXITCODE -ne 0) { throw "local quickdrop build failed" }

$remoteLocalBinary = Join-Path $binDir $remoteBinary
$oldGOOS = $env:GOOS
$oldGOARCH = $env:GOARCH
try {
  $env:GOOS = $remoteGOOS
  $env:GOARCH = $remoteGOARCH
  & $go build -o $remoteLocalBinary "$root\cmd\quickdrop"
  if ($LASTEXITCODE -ne 0) { throw "remote quickdrop cross-build failed" }
} finally {
  $env:GOOS = $oldGOOS
  $env:GOARCH = $oldGOARCH
}

$devices = @(
  @{ id = "laptop"; display_name = "Laptop"; token = "dev-laptop-token" },
  @{ id = "workstation"; display_name = "Workstation"; token = "dev-workstation-token" },
  @{ id = "main-server"; display_name = "Main Server"; token = "dev-main-server-token" }
)

$hubJson = Join-Path $cfgDir "remote-hub.json"
Write-Json $hubJson @{
  role = "hub"
  device = $devices[2]
  hub = @{
    listen = "127.0.0.1:$RemotePort"
    data_dir = "./data/hub"
    max_upload_bytes = 1073741824
  }
  devices = $devices
  groups = @(@{
    id = "all"
    name = "All Devices"
    members = @("laptop", "workstation", "main-server")
  })
}

$baseURL = "http://127.0.0.1:$LocalPort"
$laptopJson = Join-Path $cfgDir "laptop.json"
$workstationJson = Join-Path $cfgDir "workstation.json"
Write-Json $laptopJson @{
  role = "agent"
  device = $devices[0]
  agent = @{
    listen = "127.0.0.1:48992"
    data_dir = (Join-Path $work "data\laptop")
    downloads_dir = (Join-Path $work "data\laptop\downloads")
  }
  hub_client = @{ base_url = $baseURL; sse_url = "$baseURL/api/events"; use_ssh_tunnel = $false }
  ssh_tunnel = @{ enabled = $false; ssh_host = $HostName; local_port = $LocalPort; remote_host = "127.0.0.1"; remote_port = $RemotePort }
  gui = @{ listen = "127.0.0.1:48994" }
}
Write-Json $workstationJson @{
  role = "agent"
  device = $devices[1]
  agent = @{
    listen = "127.0.0.1:48993"
    data_dir = (Join-Path $work "data\workstation")
    downloads_dir = (Join-Path $work "data\workstation\downloads")
  }
  hub_client = @{ base_url = $baseURL; sse_url = "$baseURL/api/events"; use_ssh_tunnel = $false }
  ssh_tunnel = @{ enabled = $false; ssh_host = $HostName; local_port = $LocalPort; remote_host = "127.0.0.1"; remote_port = $RemotePort }
  gui = @{ listen = "127.0.0.1:48995" }
}

Invoke-SSH "rm -rf '$RemoteDir' && mkdir -p '$RemoteDir'"
& scp -q $remoteLocalBinary "${HostName}:$RemoteDir/$remoteBinary"
if ($LASTEXITCODE -ne 0) { throw "scp remote binary failed" }
& scp -q $hubJson "${HostName}:$RemoteDir/hub.json"
if ($LASTEXITCODE -ne 0) { throw "scp hub config failed" }

$remoteStart = "cd '$RemoteDir' && chmod +x '$remoteBinary' && nohup ./'$remoteBinary' hub -c hub.json > hub.out.log 2> hub.err.log < /dev/null & echo `$!"
$remotePID = (Invoke-SSH $remoteStart | Select-Object -Last 1).Trim()
Write-Host "Remote Hub PID: $remotePID"

$tunnel = $null
$laptop = $null
$workstation = $null
try {
  $forward = "$LocalPort`:127.0.0.1`:$RemotePort"
  $tunnel = Start-Process -FilePath "ssh" -ArgumentList @("-N", "-L", $forward, $HostName) -WindowStyle Hidden -PassThru
  Wait-Health $baseURL

  $laptop = Start-Process -FilePath $localExe -ArgumentList @("agent", "-c", $laptopJson) -RedirectStandardOutput (Join-Path $logDir "laptop.out.log") -RedirectStandardError (Join-Path $logDir "laptop.err.log") -WindowStyle Hidden -PassThru
  $workstation = Start-Process -FilePath $localExe -ArgumentList @("agent", "-c", $workstationJson) -RedirectStandardOutput (Join-Path $logDir "workstation.out.log") -RedirectStandardError (Join-Path $logDir "workstation.err.log") -WindowStyle Hidden -PassThru
  Start-Sleep -Seconds 1

  & $localExe text -c $laptopJson device:workstation "remote hub smoke laptop to workstation"
  if ($LASTEXITCODE -ne 0) { throw "laptop text send failed" }
  Wait-Log (Join-Path $logDir "workstation.err.log") "remote hub smoke laptop to workstation"

  $payload = Join-Path $work "remote-smoke-payload.txt"
  Set-Content -Encoding UTF8 -Path $payload -Value "remote hub smoke file"
  & $localExe send -c $workstationJson device:laptop $payload
  if ($LASTEXITCODE -ne 0) { throw "workstation file send failed" }
  Wait-Log (Join-Path $logDir "laptop.err.log") "remote-smoke-payload.txt"

  Write-Host "Remote Hub smoke passed through $HostName via local tunnel $LocalPort -> $RemotePort"
} finally {
  if ($laptop) { Stop-Process -Id $laptop.Id -Force -ErrorAction SilentlyContinue }
  if ($workstation) { Stop-Process -Id $workstation.Id -Force -ErrorAction SilentlyContinue }
  if ($tunnel) { Stop-Process -Id $tunnel.Id -Force -ErrorAction SilentlyContinue }
  if ($remotePID) {
    try { Invoke-SSH "kill $remotePID" | Out-Null } catch { Write-Host "remote kill failed: $_" -ForegroundColor Yellow }
  }
  if (!$KeepRemote) {
    try { Invoke-SSH "rm -rf '$RemoteDir'" | Out-Null } catch { Write-Host "remote cleanup failed: $_" -ForegroundColor Yellow }
  }
}
