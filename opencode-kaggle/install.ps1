# Instala opencode-kaggle en Windows.
#
# - Si ya tienes OpenCode, copia tu opencode.exe (perfil aislado).
# - Si no lo tienes, descarga el binario oficial para tu sistema.
# - Crea la configuración y el lanzador.
#
# Uso:
#   powershell -ExecutionPolicy Bypass -File .\install.ps1 -BaseUrl https://tu-tunel/v1
#
[CmdletBinding()]
param(
  [string]$BaseUrl = $env:KAGGLE_BASE_URL,
  [string]$Model = $env:KAGGLE_MODEL,
  [string]$ApiKey = $env:KAGGLE_API_KEY,
  [switch]$Download,
  [string]$Prefix
)

$ErrorActionPreference = "Stop"

function Die([string]$Message) {
  Write-Host "error: $Message" -ForegroundColor Red
  exit 1
}

if (-not $Model) { $Model = "Qwen3.8-27B" }
if (-not $Prefix) { $Prefix = Join-Path $HOME ".local\bin" }

if (-not $BaseUrl) {
  $BaseUrl = Read-Host "URL del endpoint compatible con OpenAI (https://.../v1)"
}
if (-not $BaseUrl) { Die "falta -BaseUrl" }
if ($BaseUrl -notmatch '^https?://') { Die "la URL debe empezar por http:// o https://" }

$Root = Join-Path $HOME ".local\share\opencode-kaggle"
$Conf = Join-Path $HOME ".config\opencode-kaggle"
$BinDir = Join-Path $Root "bin"
$Bin = Join-Path $BinDir "opencode.exe"
New-Item -ItemType Directory -Force -Path $BinDir, $Conf, $Prefix | Out-Null

# Arquitectura: x64 o arm64.
$arch = "x64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $arch = "arm64" }

# 1. Binario de OpenCode.
$src = $env:OPENCODE_BIN
if (-not $Download -and -not $src) {
  $cmd = Get-Command opencode -ErrorAction SilentlyContinue
  if ($cmd) { $src = $cmd.Source }
}
if (-not $Download -and -not $src) {
  $candidate = Join-Path $HOME ".opencode\bin\opencode.exe"
  if (Test-Path $candidate) { $src = $candidate }
}

if (-not $Download -and $src -and (Test-Path $src)) {
  Copy-Item $src $Bin -Force
  Write-Host "OpenCode: $src"
} else {
  $url = "https://opencode.ai/update/api/latest/cli/opencode/opencode-windows-$arch.zip"
  $tmp = Join-Path ([IO.Path]::GetTempPath()) ("opencode-" + [Guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Path $tmp | Out-Null
  $zip = Join-Path $tmp "opencode.zip"
  Write-Host "Descargando OpenCode para windows-$arch..."
  Write-Host "  $url"
  Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
  Expand-Archive -Path $zip -DestinationPath (Join-Path $tmp "x") -Force
  $exe = Get-ChildItem (Join-Path $tmp "x") -Recurse -Filter "opencode.exe" | Select-Object -First 1
  if (-not $exe) { Die "no encontré opencode.exe en el archivo descargado" }
  Copy-Item $exe.FullName $Bin -Force
  Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

if (-not (Test-Path $Bin)) { Die "no se pudo preparar el binario en $Bin" }
& $Bin --version

# 2. Lanzador y configuración.
Copy-Item (Join-Path $PSScriptRoot "bin\opencode-kaggle.ps1") (Join-Path $Prefix "opencode-kaggle.ps1") -Force
Copy-Item (Join-Path $PSScriptRoot "bin\opencode-kaggle.cmd") (Join-Path $Prefix "opencode-kaggle.cmd") -Force
Copy-Item (Join-Path $PSScriptRoot "config\cli.json") (Join-Path $Conf "cli.json") -Force

$confPath = Join-Path $Conf "opencode.json"
if (Test-Path $confPath) {
  Copy-Item $confPath "$confPath.bak" -Force
  Write-Host "copia de la configuración anterior: $confPath.bak"
}

$cfg = Get-Content (Join-Path $PSScriptRoot "config\opencode.json") -Raw | ConvertFrom-Json
$cfg.model = "kaggle/$Model"
$cfg.providers.kaggle.settings.baseURL = $BaseUrl

$keyPath = Join-Path $Conf "api-key"
$cfg.providers.kaggle.settings.apiKey = "{file:" + ($keyPath -replace '\\', '/') + "}"

$placeholder = $cfg.providers.kaggle.models.'__MODEL__'
$placeholder.name = $Model
$models = New-Object PSObject
$models | Add-Member -MemberType NoteProperty -Name $Model -Value $placeholder
$cfg.providers.kaggle.models = $models

$json = $cfg | ConvertTo-Json -Depth 20
[IO.File]::WriteAllText($confPath, $json, (New-Object Text.UTF8Encoding($false)))

# 3. API key (archivo aparte, sin BOM ni salto de línea).
$haveKey = (Test-Path $keyPath) -and ((Get-Item $keyPath).Length -gt 0)
if (-not $ApiKey -and -not $haveKey) {
  $sec = Read-Host -AsSecureString "API key del endpoint (Enter para dejarla vacía)"
  $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec)
  try {
    $ApiKey = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
  }
}
if ($ApiKey) {
  [IO.File]::WriteAllText($keyPath, $ApiKey, (New-Object Text.UTF8Encoding($false)))
  Write-Host "API key guardada en $keyPath"
  try { icacls $keyPath /inheritance:r /grant:r "$($env:USERNAME):(R,W)" | Out-Null } catch {}
} elseif (-not $haveKey) {
  [IO.File]::WriteAllText($keyPath, "", (New-Object Text.UTF8Encoding($false)))
  Write-Host "AVISO: no hay API key. Guárdala luego con: opencode-kaggle key" -ForegroundColor Yellow
}

# 4. Servicio de fondo en un puerto propio.
$port = Get-Random -Minimum 49152 -Maximum 65000
$env:OPENCODE_CONFIG_DIR = $Conf
$env:OPENCODE_DB = Join-Path $Root "opencode.db"
$env:XDG_CONFIG_HOME = Join-Path $Conf "xdg-config"
$env:XDG_DATA_HOME = Join-Path $Root "xdg-data"
$env:XDG_STATE_HOME = Join-Path $HOME ".local\state\opencode-kaggle"
$env:XDG_CACHE_HOME = Join-Path $HOME ".cache\opencode-kaggle"
try { & $Bin service set port $port | Out-Null } catch {}

# 5. PATH de usuario.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathAdded = $false
if (-not $userPath -or (($userPath -split ';') -notcontains $Prefix)) {
  $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $Prefix } else { $userPath.TrimEnd(';') + ";" + $Prefix }
  [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
  $pathAdded = $true
}

Write-Host ""
Write-Host "Listo."
Write-Host ""
Write-Host "  lanzador   $(Join-Path $Prefix 'opencode-kaggle.cmd')"
Write-Host "  config     $confPath"
Write-Host "  API key    $keyPath"
Write-Host "  sesiones   $(Join-Path $Root 'opencode.db')"
Write-Host ""
if ($pathAdded) {
  Write-Host "Añadí $Prefix al PATH de usuario: abre una terminal nueva para usarlo." -ForegroundColor Yellow
}
Write-Host "Prueba:"
Write-Host "  opencode-kaggle models"
Write-Host "  opencode-kaggle run `"Responde solo: OK`""
