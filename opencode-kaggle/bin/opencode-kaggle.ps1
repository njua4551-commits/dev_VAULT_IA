# Lanzador de opencode-kaggle para Windows.
# Lo invoca el shim opencode-kaggle.cmd.
$ErrorActionPreference = "Stop"

$Root = Join-Path $HOME ".local\share\opencode-kaggle"
$Conf = Join-Path $HOME ".config\opencode-kaggle"
$Bin = Join-Path $Root "bin\opencode.exe"

function Show-Usage {
  @"
opencode-kaggle — OpenCode aislado contra un modelo de Kaggle

Uso:
  opencode-kaggle                 abre la interfaz
  opencode-kaggle run "pregunta"  sin interfaz
  opencode-kaggle models          modelos disponibles
  opencode-kaggle vault           configura Kaggle, tuneles y GPU
  opencode-kaggle up              configura si hace falta, enciende y conecta
  opencode-kaggle down            apaga el notebook
  opencode-kaggle url URL         cambia la URL del endpoint
  opencode-kaggle key             guarda la API key (se pide por teclado)
  opencode-kaggle help            esta ayuda

Cualquier otro argumento se pasa tal cual a OpenCode.

Archivos:
  configuración  $Conf\opencode.json
  API key        $Conf\api-key
  sesiones       $Root\opencode.db
"@
}

$command = if ($args.Count -gt 0) { [string]$args[0] } else { "" }
$env:OPENCODE_VAULT_LAUNCHER = $PSCommandPath
$env:OPENCODE_CONFIG_DIR = $Conf
if ($command -in @("vault", "up", "down")) {
  $action = $command
  if ($command -eq "vault") { $action = if ($args.Count -gt 1) { $args[1] } else { "menu" } }
  $script = Join-Path $Root "vault\vault_ui.py"
  $python = if (Get-Command py -ErrorAction SilentlyContinue) { "py" }
            elseif ((Get-Command python -ErrorAction SilentlyContinue).Source -notlike "*\Microsoft\WindowsApps\*" -and (Get-Command python -ErrorAction SilentlyContinue)) { "python" }
  if (-not $python) {
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) { throw "Instala Python 3.11+ y vuelve a abrir la terminal." }
    if ((Read-Host "El Vault necesita Python. Instalar Python 3.12 con winget ahora? [s/N]") -notin @("s","si","y","yes")) { exit 1 }
    & winget install --id Python.Python.3.12 --exact --scope user
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $python = Join-Path $env:LocalAppData "Programs\Python\Python312\python.exe"
    if (-not (Test-Path $python)) { throw "Python instalado. Abre una nueva terminal y repite opencode_n vault." }
  }
  if ($python -eq "py") { & py -3 $script $action } else { & $python $script $action }
  exit $LASTEXITCODE
}

if ($command -eq "help") {
  Show-Usage
  exit 0
}

if ($command -eq "url") {
  if ($args.Count -ne 2) {
    Write-Host "uso: opencode-kaggle url https://tu-endpoint/v1" -ForegroundColor Red
    exit 2
  }
  $confPath = Join-Path $Conf "opencode.json"
  $cfg = Get-Content $confPath -Raw | ConvertFrom-Json
  if (-not $cfg.providers.kaggle) { throw "Para añadir Kaggle por primera vez: .\install.ps1 -BaseUrl URL" }
  $cfg.providers.kaggle.settings.baseURL = [string]$args[1]
  $json = $cfg | ConvertTo-Json -Depth 20
  [IO.File]::WriteAllText($confPath, $json, (New-Object Text.UTF8Encoding($false)))
  Write-Host "URL actualizada: $($args[1])"
  exit 0
}

if ($command -eq "key") {
  $sec = Read-Host -AsSecureString "API key"
  $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec)
  try {
    $apiKey = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
  }
  if ([string]::IsNullOrWhiteSpace($apiKey)) {
    Write-Host "no se guardó nada (key vacía)" -ForegroundColor Red
    exit 2
  }
  $keyPath = Join-Path $Conf "api-key"
  [IO.File]::WriteAllText($keyPath, $apiKey, (New-Object Text.UTF8Encoding($false)))
  try { icacls $keyPath /inheritance:r /grant:r "$($env:USERNAME):(R,W)" | Out-Null } catch {}
  Write-Host "API key guardada en $keyPath"
  exit 0
}

if (-not (Test-Path $Bin)) {
  Write-Host "opencode-kaggle: no encuentro el binario en $Bin" -ForegroundColor Red
  Write-Host "Ejecuta install.ps1 del repositorio." -ForegroundColor Red
  exit 1
}

# Perfil aislado: todo lo que OpenCode guarda va a estas carpetas.
$env:OPENCODE_CONFIG_DIR = $Conf
$env:OPENCODE_DB = Join-Path $Root "opencode.db"
$env:XDG_CONFIG_HOME = Join-Path $Conf "xdg-config"
$env:XDG_DATA_HOME = Join-Path $Root "xdg-data"
$env:XDG_STATE_HOME = Join-Path $HOME ".local\state\opencode-kaggle"
$env:XDG_CACHE_HOME = Join-Path $HOME ".cache\opencode-kaggle"
$env:OPENCODE_DISABLE_PROJECT_CONFIG = "1"
$env:OPENCODE_CONFIG_PROJECT_DISABLE = "1"
$env:OPENCODE_DISABLE_AUTOUPDATE = "1"

# Mantener el mismo servicio para el chat y Pair device.
& $Bin @args
exit $LASTEXITCODE
