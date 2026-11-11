<#
Autoedicion segura de Nosecode en Windows.
  .\safe-edit.ps1 checkpoint           guarda el estado actual en git y muestra el hash base
  .\safe-edit.ps1 verify <hash-base>   compila a dist\nosecode-FECHA.exe (sin tocar el binario en uso),
                                        corre go build ./... y go test ./...; si algo falla,
                                        revierte el arbol de trabajo al hash base.
#>
param(
    [Parameter(Position = 0)][string]$Mode = "",
    [Parameter(Position = 1)][string]$Base = ""
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

function Show-Usage {
    Write-Host "Uso:"
    Write-Host "  .\safe-edit.ps1 checkpoint"
    Write-Host "  .\safe-edit.ps1 verify <hash-base>"
    exit 2
}

switch ($Mode) {
    "checkpoint" {
        if (git status --porcelain) {
            git add -A
            git commit -m "Checkpoint antes de autoedicion ($(Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))"
        }
        git rev-parse HEAD
    }
    "verify" {
        if (-not $Base) {
            Write-Error "Falta el hash base; usa: .\safe-edit.ps1 verify <hash-base>"
            exit 2
        }
        New-Item -ItemType Directory -Force -Path dist | Out-Null
        $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
        $out = "dist\nosecode-$stamp.exe"

        Write-Host "Nosecode: compilando binario nuevo en $out (no se toca el que esta corriendo)..."
        go build -trimpath -o $out ./cmd/nosecode
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Fallo el build; revirtiendo a $Base"
            git reset --hard $Base
            git clean -fd -- internal cmd
            exit 1
        }

        Write-Host "Nosecode: go build ./..."
        go build ./...
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Fallo go build; revirtiendo a $Base"
            git reset --hard $Base
            git clean -fd -- internal cmd
            exit 1
        }

        Write-Host "Nosecode: go test ./..."
        go test ./...
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Fallo go test; revirtiendo a $Base"
            git reset --hard $Base
            git clean -fd -- internal cmd
            exit 1
        }

        Write-Host "OK: binario nuevo en $out; build y tests pasan."
    }
    default { Show-Usage }
}
