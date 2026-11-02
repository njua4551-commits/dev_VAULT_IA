$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$bin = Join-Path $env:LOCALAPPDATA "Nosecode\bin"

& (Join-Path $root "build.ps1")

New-Item -ItemType Directory -Force -Path $bin | Out-Null
Copy-Item (Join-Path $root "dist\nosecode.exe") (Join-Path $bin "nosecode.exe") -Force

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ([string]::IsNullOrWhiteSpace($userPath)) { $userPath = "" }
$parts = $userPath -split ";" | Where-Object { $_ -ne "" }
if ($parts -notcontains $bin) {
    $newPath = ($parts + $bin) -join ";"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "Añadido al PATH de usuario: $bin"
    Write-Host "Abre una terminal NUEVA y escribe: nosecode"
} else {
    Write-Host "Ya estaba en el PATH: $bin"
    Write-Host "Ejecuta: nosecode"
}

Write-Host ""
Write-Host "Binario instalado en: $bin\nosecode.exe"
