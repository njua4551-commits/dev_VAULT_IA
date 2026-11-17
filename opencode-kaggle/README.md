# opencode-kaggle

OpenCode ejecutado con un **perfil aislado** que apunta a un modelo servido en
**Kaggle** a través de un endpoint compatible con OpenAI (por ejemplo
`llama-server` de llama.cpp, vLLM u Ollama expuestos con un túnel).

No modifica tu OpenCode normal: usa su propio binario copiado, configuración,
base de datos de sesiones, caché y servicio de fondo.

> OpenCode is [MIT-licensed](https://github.com/sst/opencode) software. This
> repository only contains configuration and a small launcher; it does not
> redistribute OpenCode, models, API keys or third-party MCP servers.

## Qué hace

- Copia el binario de OpenCode que ya tengas instalado a
  `~/.local/share/opencode-kaggle/bin/opencode`.
- Crea un perfil aparte en `~/.config/opencode-kaggle`.
- Registra un único proveedor (`kaggle`) y bloquea todos los demás, para que no
  aparezcan modelos de otros proveedores.
- Aísla `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME` y
  `XDG_CACHE_HOME`, y desactiva la configuración e instrucciones del proyecto.
- Usa un servidor privado (`--standalone`) y un puerto de servicio propio.

## Requisitos

- Linux o macOS con `bash` y `python3`.
- OpenCode instalado (v2 probado con 2.3.0): <https://opencode.ai/docs/>
- Un endpoint compatible con OpenAI que sirva el modelo, por ejemplo:

  ```text
  https://tu-tunel.ngrok-free.dev/v1
  ```

  y su API key.

## Instalar

Desde la raíz de este repositorio:

```bash
cd opencode-kaggle
./install.sh --base-url https://tu-tunel.ngrok-free.dev/v1 --model Qwen3.8-27B
```

Te pedirá la API key si no la pasas con `--api-key`. También puedes usar
variables de entorno:

```bash
KAGGLE_BASE_URL=https://tu-tunel.ngrok-free.dev/v1 \
KAGGLE_MODEL=Qwen3.8-27B \
./install.sh
```

Opciones:

| Opción | Por defecto | Qué hace |
|---|---|---|
| `--base-url URL` | — | endpoint compatible con OpenAI (obligatorio) |
| `--model NOMBRE` | `Qwen3.8-27B` | id del modelo que sirve el endpoint |
| `--api-key KEY` | se pregunta | API key (se guarda con permisos 600) |
| `--prefix DIR` | `~/.local` | dónde instalar el lanzador |

El binario se toma de `OPENCODE_BIN`, de `opencode` en el `PATH` o de
`~/.opencode/bin/opencode`.

## Uso

```bash
opencode-kaggle                    # interfaz TUI
opencode-kaggle run "pregunta"     # sin interfaz
opencode-kaggle models             # solo debe aparecer kaggle/<modelo>
opencode-kaggle url https://otro-tunel/v1
opencode-kaggle key                # guarda o cambia la API key
```

Cualquier otro argumento se pasa a OpenCode:

```bash
opencode-kaggle run --auto "usa tus herramientas para ..."
opencode-kaggle session list
```

## Archivos

| Qué | Dónde |
|---|---|
| Lanzador | `~/.local/bin/opencode-kaggle` |
| Binario copiado | `~/.local/share/opencode-kaggle/bin/opencode` |
| Configuración | `~/.config/opencode-kaggle/opencode.json` |
| API key (600) | `~/.config/opencode-kaggle/api-key` |
| Sesiones | `~/.local/share/opencode-kaggle/opencode.db` |

## Cambiar modelo o límites

Edita `~/.config/opencode-kaggle/opencode.json`:

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
| `no encuentro el binario` | vuelve a ejecutar `./install.sh` |

## Desinstalar

```bash
./uninstall.sh
```

Borra el lanzador, el perfil, las sesiones y la caché. No toca tu OpenCode
normal.

## Licencia

MIT. Ver [LICENSE](LICENSE).

OpenCode es un proyecto independiente bajo licencia MIT
(<https://github.com/sst/opencode>). Este repositorio no lo redistribuye.
