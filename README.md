# Nosecode + Dev Vault

Cliente de terminal (TUI) para modelos OpenAI-compatibles y servidor **llama.cpp**
en una GPU gratuita de Kaggle, con túneles para usarlo desde tu PC.

- **Nosecode**: chat con streaming, sesiones locales, temas, comandos `/up`, `/down`, `/urls` y `/status`.
- **Dev Vault**: enciende/apaga un notebook de Kaggle que descarga Qwen y expone `llama-server` por ngrok/Tailscale.
- **Sin secretos en el repo**: la configuración real vive en `~/.dev-vault/config.json` y `~/.nosecode/config.json`.

## Estructura

```
CLI_Nosecode/          Cliente Go (Bubble Tea)
dev-vault/             Cliente Python + notebook de Kaggle
  client/vault.py      init / up / down / url / urls / status / chat / save-binary
  kaggle/notebooks/    Notebook que arranca llama-server
  configs/             Ejemplos de configuración
  instructions/        Guías de Kaggle, ngrok y Tailscale
```

## Requisitos

- **Go 1.24+** para compilar Nosecode.
- **Python 3.11+** y el paquete `kaggle` (`python -m pip install kaggle`).
- Cuenta de **Kaggle** con token de API (`~/.kaggle/access_token` o `kaggle.json`).
- Cuenta gratuita de **ngrok** (recomendado: soporta streaming SSE).
- Opcional: **Tailscale** como segundo túnel.

## 1. Instalar Nosecode

Windows:

```powershell
cd CLI_Nosecode
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

Linux/macOS:

```sh
cd CLI_Nosecode
chmod +x install.sh build.sh
./install.sh
```

Comprobar:

```powershell
go build ./...
go test ./...
nosecode --version
```

## 2. Configurar el Dev Vault

Desde `dev-vault/`:

```powershell
python -m pip install kaggle
python client/vault.py init --user TU_USUARIO_KAGGLE
```

`init` pide (o acepta por flags):

- `--ngrok-token` y `--ngrok-domain`: authtoken y dominio gratuito de ngrok.
- `--ts-authkey`: auth key de Tailscale (opcional).
- `--no-cloudflared`: no abrir Cloudflare (no soporta SSE).
- `--idle-minutes` y `--max-hours`: apagado por inactividad y tope de sesión.

Genera `~/.dev-vault/config.json` y publica un Dataset privado `vault-config`
con la configuración para el notebook. **No subas ese archivo al repo.**

## 3. Primer encendido

```powershell
python client/vault.py up
```

La primera vez compila `llama.cpp` en Kaggle (~25–30 min) y descarga el modelo
(~19 GB). Cuando termine imprime `VAULT_URL=https://...`.

Para no volver a compilar:

```powershell
python client/vault.py down
python client/vault.py save-binary   # sube llama-server como Dataset privado
python client/vault.py up            # ahora reutiliza el binario
```

El notebook incluye el arreglo para el error `CUDA::cuda_driver` de Kaggle
(stub de `libcuda.so` + reintento con `-DGGML_CUDA_NO_VMM=ON`).

## 4. Usar Nosecode

```powershell
nosecode
```

Dentro de la TUI:

- `/vault` importa la URL, la API key y el modelo desde `~/.dev-vault`.
- Escribe un mensaje: la respuesta llega en streaming y `esc` cancela.
- `/up`, `/down`, `/urls` y `/status` ejecutan `vault.py` sin congelar la interfaz.
- `/mcp import` copia tus servidores MCP desde `~/.config/opencode/opencode.json`.
- `/mcp tools` muestra las herramientas conectadas; `/agent on|off` activa el agente.
- Arrastra el ratón para seleccionar y copiar; `ctrl+y` o `ctrl+c` copian la última respuesta.
- `F8` pide confirmación y apaga la IA en Kaggle. `ctrl+q` sale de Nosecode.

## Agente con MCP

Nosecode puede usar herramientas de servidores MCP locales (stdio) y remotos
(Streamable HTTP con cabeceras). Al enviar un mensaje en modo Build:

1. Conecta los servidores configurados (máximo ~8 s por servidor).
2. Envía al modelo el catálogo de herramientas.
3. Si el modelo pide una herramienta, muestra un modal para permitir una vez,
   permitir en la sesión o denegar.
4. Devuelve el resultado real al modelo y continúa hasta la respuesta final.

El permiso se controla en Ajustes → Permisos → Herramientas MCP (`ask`, `allow`,
`deny`) o con `/mcp ask|allow|deny`. Las imágenes que devuelven las herramientas
se guardan en `~/.nosecode/tool-images`.

Si Nosecode no encuentra `vault.py`, define la ruta completa:

```powershell
setx VAULT_SCRIPT "C:\ruta\a\dev-vault\client\vault.py"
```

## Túneles y streaming

- **ngrok**: recomendado; soporta SSE.
- **Tailscale Funnel**: alternativa; también soporta SSE.
- **Cloudflare Quick Tunnel**: solo respaldo; **no** soporta SSE.

Nosecode prueba ngrok y luego Tailscale usando `state.json`; un error 401
(API key incorrecta) no cambia de túnel.

## Seguridad

- No publiques `~/.dev-vault/config.json`, `state.json` ni `~/.kaggle/access_token`.
- Rota el authtoken de ngrok y la auth key de Tailscale si alguna vez se filtraron.
- La API key de `vault.py` protege `llama-server`; trátala como contraseña.
- Revisa `.gitignore` antes de hacer el primer commit.

## Problemas comunes

| Síntoma | Causa / solución |
| --- | --- |
| `CUDA::cuda_driver` no encontrado | Ya corregido en el notebook; si usas una copia vieja, aplica el stub de `libcuda.so`. |
| El kernel queda `CANCEL_ACKNOWLEDGED` | Kaggle cancela sesiones inactivas; evita túneles que se cuelguen (usa `tunnel: "ngrok"`). |
| `API key incorrecta (401)` | La clave de `config.json` no coincide con la del notebook; vuelve a ejecutar `init`/`up --sync-config`. |
| No hay URL | Revisa `python client/vault.py logs -f`; el modelo puede seguir descargando. |

## Licencia

MIT. Ver `LICENSE`.
