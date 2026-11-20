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
2. **Con tu propio Kaggle:** deja vacía la URL, abre `opencode_n` y escribe `/up`. La primera vez abre el asistente: instala el cliente Kaggle en un entorno Python privado, valida tu token, pide ngrok y/o Tailscale sin mostrar los secretos, genera la clave de inferencia, prepara un dataset privado y lanza el notebook. Al abrir el túnel comprueba `/v1/models` y configura el perfil. Solo necesitas Python 3.11+ con `venv`/pip, una cuenta de Kaggle habilitada para GPU e Internet y una cuenta de túnel. **Si ya tienes un Vault encendido**, también puedes indicar la URL y la clave del endpoint en el instalador.

En Windows, abre una terminal nueva después de instalar. En Linux/macOS, si `~/.local/bin` no está en tu `PATH`, ejecuta `~/.local/bin/opencode_n`. Si ya existe otro `opencode_n`, se conserva: el comando `opencode-kaggle` abre el perfil de este instalador.

## Uso

```sh
opencode_n                          # interfaz
opencode_n run "Responde solo OK"   # terminal sin interfaz
opencode_n models                    # modelos disponibles
opencode_n vault                     # menú /vault sin abrir el chat
opencode_n up                        # primer encendido asistido o reconexión
opencode_n down                      # apagar para no consumir GPU
opencode_n url https://tu-tunel/v1   # actualizar un Vault ya configurado
opencode_n key                       # cambiar la clave del Vault
```

`/vault` también permite reconfigurar los tokens, consultar estado, cuota, túneles y logs. Las credenciales se introducen en una terminal local, no en un mensaje al modelo. El asistente guarda el estado bajo `~/.config/opencode-kaggle/vault/`; usa nombres privados propios por instalación. Los siguientes encendidos reutilizan esos datos. Cancelar el cliente no apaga un notebook que ya arrancó.

## Pair device en la misma red

El perfil conserva los comandos nativos de OpenCode y usa su propio servicio gestionado. Para permitir conexiones desde tu Wi-Fi, ejecuta **una vez en la PC** `opencode_n service set hostname 0.0.0.0`. Abre `/pair` dentro del chat (o `opencode_n pair` en la terminal) y abre en el celular el enlace que contiene la **IP LAN de la PC**. El enlace es de un solo uso. No necesitas ngrok para esto. Si un firewall bloquea el puerto, permite el acceso desde tu red privada. Para volver a aceptar solo conexiones locales: `opencode_n service set hostname 127.0.0.1`.

Puedes añadir Kaggle más adelante repitiendo el instalador con `--base-url URL` (Windows: `-BaseUrl URL`). También acepta `--model NOMBRE` / `-Model NOMBRE`, `--download` / `-Download` y las variables `KAGGLE_BASE_URL`, `KAGGLE_MODEL` y `KAGGLE_API_KEY` (clave de inferencia del Vault). Al actualizar, los ajustes anteriores quedan en `opencode.json.bak`.

El contexto se compacta al **80 %**. Qwen3.8-27B incluye variantes `off`, `low`, `medium` y `high`. En el perfil multiproveedor, el resumen usa el modelo de la sesión: una conversación de otro proveedor no se envía al Vault para compactarla.

## Archivos y desinstalación

La configuración y la clave del Vault están bajo `~/.config/opencode-kaggle/`; el binario y las sesiones, bajo `~/.local/share/opencode-kaggle/`. En Windows `~` es tu carpeta de usuario. Para añadir un MCP propio, usa `/mcps` o la [guía de MCP](https://opencode.ai/docs/mcp-servers).

`bash uninstall.sh` o `powershell -ExecutionPolicy Bypass -File .\uninstall.ps1` eliminan este perfil, incluidas sus sesiones. OpenCode se distribuye por separado; el instalador y lanzador tienen [licencia MIT](LICENSE).
