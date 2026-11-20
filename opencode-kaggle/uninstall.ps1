# Desinstala opencode-kaggle en Windows: lanzador, perfil, sesiones y caché.
[CmdletBinding()]
param([switch]$Yes)

$ErrorActionPreference = "Stop"
$Prefix = Join-Path $HOME ".local\bin"
$Root = Join-Path $HOME ".local\share\opencode-kaggle"
$Conf = Join-Path $HOME ".config\opencode-kaggle"
$State = Join-Path $HOME ".local\state\opencode-kaggle"
$Cache = Join-Path $HOME ".cache\opencode-kaggle"

if (-not $Yes) {
  Write-Host "Se borrará:"
  Write-Host "  $(Join-Path $Prefix 'opencode-kaggle.cmd')"
  Write-Host "  $(Join-Path $Prefix 'opencode-kaggle.ps1')"
  Write-Host "  $Conf"
  Write-Host "  $Root"
  Write-Host "  $State"
  Write-Host "  $Cache"
  $ok = Read-Host "¿Continuar? [s/N]"
  if ($ok -notmatch '^[sS]') {
    Write-Host "cancelado"
    exit 1
  }
}

$aliasPath = Join-Path $Prefix "opencode_n.cmd"
$launcher = Join-Path $Prefix "opencode-kaggle.cmd"
if (Test-Path $launcher) { & $launcher service stop | Out-Null }
if ((Test-Path $aliasPath) -and (Test-Path $launcher) -and (Get-FileHash $aliasPath).Hash -eq (Get-FileHash $launcher).Hash) {
  Remove-Item $aliasPath -Force
}
Remove-Item $launcher, (Join-Path $Prefix "opencode-kaggle.ps1") -Force -ErrorAction SilentlyContinue
Remove-Item $Conf, $Root, $State, $Cache -Recurse -Force -ErrorAction SilentlyContinue

# Quita el lanzador del PATH de usuario.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath) {
  $parts = $userPath -split ';' | Where-Object { $_ -and $_ -ne $Prefix }
  [Environment]::SetEnvironmentVariable("Path", ($parts -join ';'), "User")
}

Write-Host "desinstalado"
