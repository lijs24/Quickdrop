param(
  [string[]]$Hosts = @()
)

$ErrorActionPreference = "Stop"

if ($Hosts.Count -eq 0) {
  $configPath = Join-Path $env:USERPROFILE ".ssh\config"
  if (!(Test-Path $configPath)) {
    throw "No SSH config found at $configPath and no -Hosts were provided."
  }
  $Hosts = Get-Content $configPath |
    ForEach-Object { $_.Trim() } |
    Where-Object { $_ -match '^Host\s+' -and $_ -notmatch '[*?]' } |
    ForEach-Object { ($_ -replace '^Host\s+', '').Split(' ', [System.StringSplitOptions]::RemoveEmptyEntries) } |
    Select-Object -Unique
} else {
  $Hosts = $Hosts |
    ForEach-Object { $_.Split(',', [System.StringSplitOptions]::RemoveEmptyEntries) } |
    ForEach-Object { $_.Trim() } |
    Where-Object { $_ -ne "" } |
    Select-Object -Unique
}

foreach ($hostName in $Hosts) {
  Write-Host "== $hostName =="
  $probe = & ssh -o BatchMode=yes -o ConnectTimeout=5 $hostName "echo quickdrop-ssh-ok" 2>&1
  if ($LASTEXITCODE -ne 0) {
    Write-Host "ssh probe failed for $hostName (exit $LASTEXITCODE)" -ForegroundColor Yellow
    $probe | ForEach-Object { Write-Host "  $_" }
    continue
  }
  $probe | ForEach-Object { Write-Host $_ }

  $os = & ssh -o BatchMode=yes -o ConnectTimeout=5 $hostName "uname -s" 2>&1
  if ($LASTEXITCODE -ne 0) {
    $os = & ssh -o BatchMode=yes -o ConnectTimeout=5 $hostName "cmd /c ver" 2>&1
  }
  if ($LASTEXITCODE -eq 0) {
    $os | ForEach-Object { Write-Host $_ }
  } else {
    Write-Host "  connected, but OS probe failed" -ForegroundColor Yellow
  }
}
