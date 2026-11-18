# opencode-kaggle

OpenCode ejecutado con un **perfil aislado** que apunta a un modelo servido en
**Kaggle** a través de un endpoint compatible con OpenAI (por ejemplo
`llama-server` de llama.cpp, vLLM u Ollama expuestos con un túnel).

No modifica tu OpenCode normal: usa su propio binario, configuración, base de
datos de sesiones, caché y servicio de fondo.

Instalador para **Linux, macOS y Windows**.

> OpenCode is [MIT-licensed](https://github.com/sst/opencode) software. This
> repository only contains configuration and a small installer/launcher; it
> does not redistribute OpenCode, models, API keys or third-party MCP servers.

## Qué hace

- Usa tu OpenCode si ya lo tienes; si no, descarga el binario oficial para tu
  sistema desde `opencode.ai` (~120 MB).
- Lo copia a `~/.local/share/opencode-kaggle/bin/` para que sea independiente.
- Crea un perfil aparte en `~/.config/opencode-kaggle`.
- Registra un único proveedor (`kaggle`) y bloquea todos los demás, para que no
  aparezcan modelos de otros proveedores.
- Aísla `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME` y
  `XDG_CACHE_HOME`, y desactiva la configuración e instrucciones del proyecto.
- Usa un servidor privado (`--standalone`) y un puerto de servicio propio.
- Deja la API key en un archivo aparte, nunca en el JSON ni en el repositorio.

## Requisitos

- **Linux/macOS:** `bash`, `python3` y `curl` o `wget` (macOS: `unzip`).
- **Windows 10/11:** PowerShell 5.1 o superior (viene incluido).
- Un endpoint compatible con OpenAI que sirva el modelo, por ejemplo:

  ```text
  https://tu-tunel.ngrok-free.dev/v1
  ```

  y su API key.

No hace falta tener OpenCode instalado: el instalador lo descarga si falta.

## Instalar

### Linux y macOS

```bash
cd opencode-kaggle
./install.sh --base-url https://tu-tunel.ngrok-free.dev/v1 --model Qwen3.8-27B
```

### Windows

```powershell
cd opencode-kaggle
powershell -ExecutionPolicy Bypass -File .\install.ps1 -BaseUrl https://tu-tunel.ngrok-free.dev/v1 -Model Qwen3.8-27B
```

Los dos piden la API key si no se la pasas. También aceptan variables de
entorno: `KAGGLE_BASE_URL`, `KAGGLE_MODEL` y `KAGGLE_API_KEY`.

Opciones equivalentes:

| Linux/macOS | Windows | Por defecto | Qué hace |
|---|---|---|---|
| `--base-url URL` | `-BaseUrl URL` | — | endpoint (obligatorio) |
| `--model NOMBRE` | `-Model NOMBRE` | `Qwen3.8-27B` | id del modelo |
| `--api-key KEY` | `-ApiKey KEY` | se pregunta | API key |
| `--download` | `-Download` | no | descarga OpenCode aunque ya exista |
| `--prefix DIR` | `-Prefix DIR` | `~/.local` | dónde va el lanzador |

Si ya tienes OpenCode, se copia de `OPENCODE_BIN`, de `opencode` en el `PATH` o
de `~/.opencode/bin/opencode`. Si no hay ninguno, se descarga la variante
correcta: Linux glibc/musl, x64/arm64, baseline si el CPU no tiene AVX2;
macOS arm64/x64; Windows x64/arm64.

## Uso

```bash
opencode-kaggle                    # interfaz TUI
opencode-kaggle run "pregunta"     # sin interfaz
opencode-kaggle models             # solo debe aparecer kaggle/<modelo>
opencode-kaggle url https://otro-tunel/v1
opencode-kaggle key                # guarda o cambia la API key
```

En Windows funciona igual desde una terminal nueva (el instalador añade el
lanzador al `PATH` de usuario).

Cualquier otro argumento se pasa a OpenCode:

```bash
opencode-kaggle run --auto "usa tus herramientas para ..."
opencode-kaggle session list
```

## Archivos

| Qué | Linux/macOS | Windows |
|---|---|---|
| Lanzador | `~/.local/bin/opencode-kaggle` | `%USERPROFILE%\.local\bin\opencode-kaggle.cmd` |
| Binario | `~/.local/share/opencode-kaggle/bin/opencode` | `%USERPROFILE%\.local\share\opencode-kaggle\bin\opencode.exe` |
| Configuración | `~/.config/opencode-kaggle/opencode.json` | `%USERPROFILE%\.config\opencode-kaggle\opencode.json` |
| API key (600) | `~/.config/opencode-kaggle/api-key` | `%USERPROFILE%\.config\opencode-kaggle\api-key` |
| Sesiones | `~/.local/share/opencode-kaggle/opencode.db` | `%USERPROFILE%\.local\share\opencode-kaggle\opencode.db` |

## Cambiar modelo o límites

Edita `~/.config/opencode-kaggle/opencode.json` (en Windows,
`%USERPROFILE%\.config\opencode-kaggle\opencode.json`):

```jsonc
"model": "kaggle/Qwen3.8-27B",
"providers": {
  "kaggle": {
    "settings": { "baseURL": "https://tu-tunel/v1" },
    "models": {
      "Qwen3.8-27B": {
        "limit": { "context": 131072, "output": 8192 }
      }
    }
  }
}
```

Los demás proveedores están bloqueados con `policies`; si quieres usar otro,
añádelo y permite su `provider.use` en esa lista.

## Añadir MCP

Este repositorio no incluye servidores MCP. Puedes añadir los tuyos en
`opencode.json`:

```jsonc
"mcp": {
  "servers": {
    "mi-mcp": {
      "type": "local",
      "command": ["node", "/ruta/a/mi-mcp/server.js"]
    }
  }
}
```

Documentación: <https://opencode.ai/docs/mcp>

## Problemas típicos

| Síntoma | Causa |
|---|---|
| `HTTP 404` o `ERR_NGROK_3200` | el túnel está caído o cambió de URL: `opencode-kaggle url ...` |
| `HTTP 401` | API key incorrecta: `opencode-kaggle key` |
| `Connection refused` | el endpoint no está escuchando o la URL no termina en `/v1` |
| No aparece el modelo | revisa el id exacto en `/v1/models` del endpoint |
| `no encuentro el binario` | vuelve a ejecutar el instalador |
| En Windows no encuentra `opencode-kaggle` | abre una terminal nueva (se actualizó el `PATH`) |
| El binario descargado no arranca en Linux | CPU sin AVX2 o musl: el instalador ya elige la variante; prueba `--download` de nuevo |

## Desinstalar

```bash
./uninstall.sh
```

```powershell
powershell -ExecutionPolicy Bypass -File .\uninstall.ps1
```

Borra el lanzador, el perfil, las sesiones y la caché. No toca tu OpenCode
normal.

## Licencia

MIT. Ver [LICENSE](LICENSE).

OpenCode es un proyecto independiente bajo licencia MIT
(<https://github.com/sst/opencode>). Este repositorio no lo redistribuye.
