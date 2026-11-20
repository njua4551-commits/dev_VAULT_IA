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
  $BaseUrl = Read-Host "Vault ya encendido: URL (Enter para configurar despues con /vault o /connect)"
}
if ($BaseUrl -and $BaseUrl -notmatch '^https?://') { Die "la URL debe empezar por http:// o https://" }

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

$v2 = $false
if (-not $Download -and $src -and (Test-Path $src)) {
  $v2 = ((& $src --version) -match '(^|\s)v?2\.')
}
if ($v2) {
  Copy-Item $src $Bin -Force
  Write-Host "OpenCode: $src"
} else {
  if ($arch -eq "x64") {
    $cpu = Add-Type -Name CpuFeature -Namespace OpenCodeKaggle -PassThru -MemberDefinition '[System.Runtime.InteropServices.DllImport("kernel32.dll")] public static extern bool IsProcessorFeaturePresent(int feature);'
    if (-not $cpu::IsProcessorFeaturePresent(40)) { $arch = "x64-baseline" }
  }
  $release = Invoke-RestMethod -Uri "https://registry.npmjs.org/@opencode%2fcli-windows-$arch/latest"
  if ($release.version -notmatch '^2\.') { Die "El instalador necesita OpenCode V2" }
  $url = $release.dist.tarball
  $tmp = Join-Path ([IO.Path]::GetTempPath()) ("opencode-" + [Guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Path $tmp | Out-Null
  $zip = Join-Path $tmp "opencode.tgz"
  Write-Host "Descargando OpenCode para windows-$arch..."
  Write-Host "  $url"
  Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
  New-Item -ItemType Directory -Path (Join-Path $tmp "x") | Out-Null
  tar.exe -xzf $zip -C (Join-Path $tmp "x")
  if ($LASTEXITCODE -ne 0) { Die "No se pudo extraer el paquete oficial" }
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
if (-not (Get-Command opencode_n -ErrorAction SilentlyContinue) -and -not (Test-Path (Join-Path $Prefix "opencode_n.cmd"))) {
  Copy-Item (Join-Path $PSScriptRoot "bin\opencode-kaggle.cmd") (Join-Path $Prefix "opencode_n.cmd")
}
Copy-Item (Join-Path $PSScriptRoot "config\cli.json") (Join-Path $Conf "cli.json") -Force
$bundle = Join-Path $Root "vault"
$plugin = Join-Path $Conf "plugins\vault"
foreach ($dir in @("$bundle\client", "$bundle\kaggle\notebooks", "$bundle\config", $plugin)) {
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
}
Copy-Item "$PSScriptRoot\vault_ui.py" $bundle -Force
Copy-Item "$PSScriptRoot\..\dev-vault\client\vault.py" "$bundle\client" -Force
Copy-Item "$PSScriptRoot\..\dev-vault\kaggle\notebooks\run-qwen-llamacpp.ipynb" "$bundle\kaggle\notebooks" -Force
Copy-Item "$PSScriptRoot\config\opencode.json","$PSScriptRoot\config\qwen-variants.json" "$bundle\config" -Force
Copy-Item "$PSScriptRoot\plugins\vault\*" $plugin -Force

$confPath = Join-Path $Conf "opencode.json"
if (Test-Path $confPath) {
  Copy-Item $confPath "$confPath.bak" -Force
  Write-Host "copia de la configuración anterior: $confPath.bak"
}

$tpl = Get-Content (Join-Path $PSScriptRoot "config\opencode.json") -Raw | ConvertFrom-Json
$cfg = if (Test-Path $confPath) { Get-Content $confPath -Raw | ConvertFrom-Json } else { [PSCustomObject]@{'$schema'=$tpl.'$schema'} }
if (-not $cfg.compaction) { $cfg | Add-Member -NotePropertyName compaction -NotePropertyValue $tpl.compaction -Force }
if ($cfg.plugins -notcontains $plugin) {
  $plugins = @($cfg.plugins | Where-Object { $_ }) + @($plugin)
  $cfg | Add-Member -NotePropertyName plugins -NotePropertyValue $plugins -Force
}
$rules = @($cfg.policies)
if ($rules.Count -eq 2 -and $rules[0].action -eq 'provider.use' -and $rules[0].resource -eq '*' -and $rules[0].effect -eq 'deny' -and
    $rules[1].action -eq 'provider.use' -and $rules[1].resource -eq 'kaggle' -and $rules[1].effect -eq 'allow' -and
    @($rules[0].PSObject.Properties).Count -eq 3 -and @($rules[1].PSObject.Properties).Count -eq 3) {
  $cfg.PSObject.Properties.Remove('policies')
}
$keyPath = Join-Path $Conf "api-key"
if ($BaseUrl) {
  $cfg | Add-Member -NotePropertyName model -NotePropertyValue "kaggle/$Model" -Force
  if (-not $cfg.providers) { $cfg | Add-Member -NotePropertyName providers -NotePropertyValue ([PSCustomObject]@{}) -Force }
  $placeholder = $tpl.providers.kaggle.models.'__MODEL__'
  $placeholder.name = $Model
  $tpl.providers.kaggle.models.PSObject.Properties.Remove('__MODEL__')
  if (-not $cfg.providers.kaggle) { $cfg.providers | Add-Member -NotePropertyName kaggle -NotePropertyValue $tpl.providers.kaggle }
  $provider = $cfg.providers.kaggle
  $provider.settings | Add-Member -NotePropertyName baseURL -NotePropertyValue $BaseUrl -Force
  $provider.settings | Add-Member -NotePropertyName apiKey -NotePropertyValue ("{file:" + ($keyPath -replace '\\', '/') + "}") -Force
  if (-not $provider.models.PSObject.Properties[$Model]) { $provider.models | Add-Member -NotePropertyName $Model -NotePropertyValue $placeholder }
  if ($Model -eq "Qwen3.8-27B") {
    $entry = $provider.models.$Model
    if (-not $entry.variants) { $entry | Add-Member -NotePropertyName variants -NotePropertyValue (Get-Content (Join-Path $PSScriptRoot 'config\qwen-variants.json') -Raw | ConvertFrom-Json) }
    if (-not $entry.capabilities.reasoning) { $entry.capabilities | Add-Member -NotePropertyName reasoning -NotePropertyValue @{supported=$true} }
    if (-not $entry.compatibility.reasoningField) { $entry.compatibility | Add-Member -NotePropertyName reasoningField -NotePropertyValue 'reasoning_content' }
  }
}

$json = $cfg | ConvertTo-Json -Depth 20
[IO.File]::WriteAllText($confPath, $json, (New-Object Text.UTF8Encoding($false)))

# 3. API key (archivo aparte, sin BOM ni salto de línea).
$haveKey = (Test-Path $keyPath) -and ((Get-Item $keyPath).Length -gt 0)
if ($BaseUrl -and -not $ApiKey -and -not $haveKey) {
  $sec = Read-Host -AsSecureString "API key del endpoint (Enter para dejarla vacía)"
  $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec)
  try {
    $ApiKey = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
  }
}
if ($BaseUrl -and $ApiKey) {
  [IO.File]::WriteAllText($keyPath, $ApiKey, (New-Object Text.UTF8Encoding($false)))
  Write-Host "API key guardada en $keyPath"
  try { icacls $keyPath /inheritance:r /grant:r "$($env:USERNAME):(R,W)" | Out-Null } catch {}
} elseif ($BaseUrl -and -not $haveKey) {
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
Write-Host "  opencode-kaggle   (/vault para Kaggle, /connect para otras APIs)"
