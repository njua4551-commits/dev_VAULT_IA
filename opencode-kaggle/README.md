# opencode_n portátil

Un perfil aislado de OpenCode V2 para **tu propio Vault de Kaggle, otros proveedores con tu API, o ambos**. No hace falta compilar OpenCode ni editar JSON. La instalación no incluye cuentas, sesiones, claves ni MCP de otra persona.

## Instalar en otra PC

[Descarga el ZIP](https://github.com/njua4551-commits/dev_VAULT_IA/archive/refs/heads/main.zip), descomprímelo y abre una terminal **dentro de `opencode-kaggle`**. También puedes clonar el repositorio con Git.

| Sistema | Comando |
|---|---|
| Linux o macOS (x64/ARM64, Bash, Python 3 y `tar`) | `bash install.sh` |
| Windows 10/11 (x64/ARM64, PowerShell y `tar.exe`) | `powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1` |

El instalador reutiliza OpenCode V2 si lo encuentra; si no, descarga el paquete oficial para tu plataforma. Necesitas Internet para descargar y para usar una API remota.

1. **Solo API propia:** deja vacía la URL de Kaggle pulsando Enter. Abre `opencode_n`, escribe `/connect`, elige tu proveedor e introduce tu clave. Después usa `/models` para elegir el modelo.
2. **Con Kaggle:** introduce la URL de **tu servidor de inferencia** (`https://tu-tunel/v1`) y la clave de **ese endpoint**, no el token de tu cuenta de Kaggle. Se selecciona Qwen3.8-27B por defecto. Puedes conectar otras APIs con `/connect` y cambiar con `/models`. Para encender tu propio Vault, consulta la [guía del servidor](../dev-vault/instructions/client-guide.md).

En Windows, abre una terminal nueva después de instalar. En Linux/macOS, si `~/.local/bin` no está en tu `PATH`, ejecuta `~/.local/bin/opencode_n`. Si ya existe otro `opencode_n`, se conserva: el comando `opencode-kaggle` abre el perfil de este instalador.

## Uso

```sh
opencode_n                          # interfaz
opencode_n run "Responde solo OK"   # terminal sin interfaz
opencode_n models                    # modelos disponibles
opencode_n url https://tu-tunel/v1   # actualizar un Vault ya configurado
opencode_n key                       # cambiar la clave del Vault
```

Puedes añadir Kaggle más adelante repitiendo el instalador con `--base-url URL` (Windows: `-BaseUrl URL`). También acepta `--model NOMBRE` / `-Model NOMBRE`, `--download` / `-Download` y las variables `KAGGLE_BASE_URL`, `KAGGLE_MODEL` y `KAGGLE_API_KEY` (clave de inferencia del Vault). Al actualizar, los ajustes anteriores quedan en `opencode.json.bak`.

El contexto se compacta al **80 %**. Qwen3.8-27B incluye variantes `off`, `low`, `medium` y `high`. En el perfil multiproveedor, el resumen usa el modelo de la sesión: una conversación de otro proveedor no se envía al Vault para compactarla.

## Archivos y desinstalación

La configuración y la clave del Vault están bajo `~/.config/opencode-kaggle/`; el binario y las sesiones, bajo `~/.local/share/opencode-kaggle/`. En Windows `~` es tu carpeta de usuario. Para añadir un MCP propio, usa `/mcps` o la [guía de MCP](https://opencode.ai/docs/mcp-servers).

`bash uninstall.sh` o `powershell -ExecutionPolicy Bypass -File .\uninstall.ps1` eliminan este perfil, incluidas sus sesiones. OpenCode se distribuye por separado; el instalador y lanzador tienen [licencia MIT](LICENSE).
