$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
$go = (Get-Command go -ErrorAction Stop).Source

Push-Location $root
try {
    Write-Host "Nosecode: compilando..."
    New-Item -ItemType Directory -Force -Path (Join-Path $root "dist") | Out-Null

    $ldflags = "-s -w -X nosecode/internal/version.Version=0.6.0 -X nosecode/internal/version.Commit=build"

    & $go build -trimpath -ldflags $ldflags -o "dist\nosecode.exe" ./cmd/nosecode
    if ($LASTEXITCODE -ne 0) { throw "fallo el build de Windows" }

    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    & $go build -trimpath -ldflags $ldflags -o "dist\nosecode-linux-amd64" ./cmd/nosecode
    if ($LASTEXITCODE -ne 0) { throw "fallo el build de Linux" }
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue

    Write-Host "Listo: dist\nosecode.exe y dist\nosecode-linux-amd64"
}
finally {
    Pop-Location
}
