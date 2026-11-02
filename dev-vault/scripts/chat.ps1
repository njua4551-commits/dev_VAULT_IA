# Cliente simple en PowerShell para la API de llama-server (Kaggle)
# Uso:
#   .\chat.ps1 -Url "https://xxxx.trycloudflare.com" -Key "TU_API_KEY" -Prompt "Hola"
#   .\chat.ps1 -Url "https://xxxx.trycloudflare.com" -Key "TU_API_KEY" -Prompt "Replica esta pantalla en HTML" -Image .\captura.png
param(
    [Parameter(Mandatory = $true)][string]$Url,
    [Parameter(Mandatory = $true)][string]$Key,
    [Parameter(Mandatory = $true)][string]$Prompt,
    [string]$Model = "Qwen3.8-27B",
    [string]$Image
)

if ($Image) {
    if (-not (Test-Path $Image)) { throw "No existe la imagen: $Image" }
    $ext = [IO.Path]::GetExtension($Image).ToLower()
    $mime = switch ($ext) { ".png" { "image/png" } ".jpg" { "image/jpeg" } ".jpeg" { "image/jpeg" } ".webp" { "image/webp" } ".gif" { "image/gif" } default { throw "Formato de imagen no soportado: $ext" } }
    $b64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes($Image))
    $content = @(
        @{ type = "text"; text = $Prompt },
        @{ type = "image_url"; image_url = @{ url = "data:$mime;base64,$b64" } }
    )
} else {
    $content = $Prompt
}

$body = @{
    model    = $Model
    messages = @(@{ role = "user"; content = $content })
    stream   = $false
} | ConvertTo-Json -Depth 8

$headers = @{ Authorization = "Bearer $Key" }

$resp = Invoke-RestMethod -Method Post `
    -Uri "$($Url.TrimEnd('/'))/v1/chat/completions" `
    -Headers $headers -ContentType "application/json" -Body $body -TimeoutSec 600

$resp.choices[0].message.content
