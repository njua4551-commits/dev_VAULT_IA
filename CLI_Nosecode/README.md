# Nosecode

Cliente de terminal (TUI) para conversar con endpoints compatibles con la API
de OpenAI. Interfaz con la misma disposición que **opencode**, tema **Vercel**,
sesiones locales y navegación con teclado o ratón.

```
                          █▀▀█ █▀▀█ █▀▀▀ █▀▀▀   █▀▀▀ █▀▀█ █▀▀▄ █▀▀▀
                          █  █ █  █ ▀▀▀█ █▀▀    █    █  █ █  █ █▀▀
                          ▀  ▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀   ▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀

              ┃
              ┃  Escribe un mensaje…
              ┃
              ┃  Build  sin modelo  vault
              ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
                                    shift+tab agentes   ctrl+p comandos
```

## Estado actual (0.4.0)

La interfaz funciona sin ningún endpoint configurado: puedes abrirla, navegar
los ajustes, crear proveedores y conservar tus conversaciones. No realiza
peticiones de red.

- Tema **Vercel** (fondo `#000000`, primario `#0070F3`) y alternativa `midnight`.
- Home como opencode: logo en bloques, prompt con barra `┃` del agente
  (Build / Plan), línea de agente · modelo · proveedor, pistas y un tip.
- Sesiones persistentes en `~/.nosecode/sessions/*.json`.
- Cabecera de sesión (`# título`, clic para renombrar) y mensajes en paneles.
- Sesiones como pestañas de navegador sobre el chat: cambiar, cerrar y crear.
- Barra lateral derecha con contexto, proveedor y sesiones: cambiar, crear y
  cerrar (también con `ctrl+p` o `/sessions`).
- Configuración en una lista plana buscable, agrupada por categorías.
- Paleta de comandos (`ctrl+p`) con categorías, atajos, sesiones y filtro.
- Ratón: barra lateral, prompt, filas, toggles, botones y paleta; rueda para
  desplazar.
- Sonidos al completar acciones, preguntar, fallar o cerrar.
- Importación del endpoint del Dev Vault (`~/.dev-vault`).
- Chat con streaming SSE y comandos `/up`, `/down`, `/urls` y `/status`.

El agente usa herramientas nativas en Go (bash, leer/escribir/editar archivos,
listar, grep, captura de pantalla, ver imágenes, descargar webs y memoria). Las
herramientas de navegador (`browser_navigate`, `browser_click`, `browser_type`)
solo se envían con `/tools browser on`; el resto de MCP, con `/tools mcp on`.
El contexto se controla con microcompact, `/compact` y un aviso al 80 % de la
ventana; `NOSECODE.md` se carga como memoria al iniciar cada sesión.

## Instalar

### Windows

```powershell
cd CLI_Nosecode
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

Añade `%LOCALAPPDATA%\Nosecode\bin` al PATH de usuario. Abre una terminal
**nueva** y escribe `nosecode`.

### Linux

```sh
cd ~/CLI_Nosecode
chmod +x install.sh build.sh
./install.sh
nosecode
```

Deja el binario en `~/.local/bin/nosecode`. Para los sonidos usa `ffplay`,
`mpg123` o `cvlc`; si no hay ninguno, suena la campana del terminal.

### Compilar a mano

`install.ps1` / `build.ps1` / `build.sh` ya recompilan, no hace falta `dist/`.

```sh
go build -o dist/nosecode ./cmd/nosecode
```

## Uso

```sh
nosecode                    # abre la TUI
nosecode --demo=home        # imprime una pantalla sin TTY
nosecode --demo=session
nosecode --demo=settings
nosecode --demo=settings-edit
nosecode --demo=palette
nosecode --config           # ruta del config
nosecode --version
```

### Teclado

| Tecla | Acción |
| --- | --- |
| `enter` | enviar prompt |
| `alt+enter` | nueva línea |
| `ctrl+n` | nueva sesión |
| `shift+tab` | alternar agente Plan / Build |
| `f2` / `ctrl+w` | renombrar / eliminar sesión |
| `ctrl+p` | paleta de comandos |
| `f1` | ajustes |
| `ctrl+b` | mostrar u ocultar el panel lateral |
| `esc` | cancelar respuesta / limpiar entrada / cerrar modal |
| `pgup` / `pgdn` | desplazar el chat |
| `ctrl+y` | copiar la última respuesta del asistente |
| `ctrl+c` | copiar la selección (nunca cierra) |
| `ctrl+q` | salir |

En los ajustes, escribe para filtrar, `↑↓` para moverte y `enter` para
cambiar. En la paleta, escribe para filtrar y `enter` para ejecutar.

### Ratón

| Zona | Acción |
| --- | --- |
| pestaña de sesión | cambiar de sesión |
| `×` de la pestaña activa | cerrar sesión |
| `+` de las pestañas o de «Sesiones» | nueva sesión |
| sesión en la barra lateral | cambiar de sesión |
| `×` de la sesión activa | cerrar sesión |
| título (cabecera o lateral) | renombrar sesión |
| agente en el prompt | alternar Plan / Build |
| modelo / proveedor | abrir ajustes |
| fila de ajuste | activar, editar o cambiar |
| botones del modal | guardar, cancelar, importar |
| arrastrar sobre el chat | seleccionar por caracteres y copiar |
| doble / triple clic | palabra / cuerpo del mensaje |
| rueda | desplazar el chat |

### Comandos

`/settings`, `/model [nombre]`, `/sessions`, `/copy [code [n]|files]`,
`/tokens`, `/sound`, `/theme`, `/tools [browser|mcp on|off]`, `/compact`,
`/mcp`, `/agent`, `/vault`, `/up`, `/down`,
`/urls`, `/status`, `/config`, `/new`, `/help`, `/quit`.

`/copy` copia la última respuesta completa y guarda sus bloques de código en
`~/.nosecode/copies`; `/copy code [n]` copia un bloque y `/copy files` los
guarda sin tocar el portapapeles.

### Chat con streaming

Con un proveedor configurado (`/vault` importa el del Dev Vault) los mensajes
se envían por SSE a `POST {base_url}/chat/completions`: los tokens aparecen al
llegar y `esc` cancela. Si el túnel falla (conexión, 402/403/429/5xx) prueba los
otros túneles de `~/.dev-vault/state.json` (ngrok, Tailscale; Cloudflare se
omite por no soportar SSE). Un 401 se muestra como API key incorrecta.

`/up`, `/down`, `/urls` y `/status` ejecutan `client/vault.py` y muestran su
salida en vivo. Localiza el script con `VAULT_SCRIPT` (ruta completa), la
carpeta actual o una copia local de `dev-vault`.

## Sesiones

Cada conversación se guarda en `~/.nosecode/sessions/<id>.json` con permisos
`0600`. Al arrancar se retoma la sesión usada más recientemente y todas
aparecen como pestañas, en la barra lateral y en `/sessions`. Cerrar una sesión borra su archivo; siempre
queda al menos una sesión abierta.

`NOSECODE_HOME` cambia la carpeta de datos.

## Zoom del terminal

El zoom con `ctrl` + rueda lo controla el emulador de terminal, no la
aplicación. Si el texto queda demasiado grande o pequeño, pulsa `ctrl+0`
(Windows Terminal también admite `ctrl+shift+0`) para restablecerlo. Nosecode
muestra un aviso cuando la ventana es demasiado pequeña y mantiene el diseño
estable en cualquier tamaño.

## Configuración

Se crea sola en `~/.nosecode/config.json` (o
`%USERPROFILE%\.nosecode\config.json`):

```json
{
  "version": 2,
  "default_provider": "vault",
  "providers": {
    "vault": {
      "type": "openai",
      "base_url": "https://tu-tunel.ngrok.app/v1",
      "api_key": "tu-key",
      "model": "Qwen3.8-27B",
      "context_window": 131072
    }
  },
  "permissions": { "bash": "ask", "edit": "ask", "webfetch": "ask" },
  "tools": { "browser": false, "mcp": false, "compact_keep": 6 },
  "ui": {
    "theme": "vercel",
    "sidebar": true,
    "animations": true,
    "tokens": true,
    "mute": false
  }
}
```

- `context_window` es el valor de reserva (131072 por defecto). Si el servidor
  expone `/props` con `n_ctx`, Nosecode usa ese valor al arrancar.
- `tools.browser` y `tools.mcp` también se cambian con `/tools browser on|off`
  y `/tools mcp on|off`; `compact_keep` es cuántas salidas de herramientas se
  conservan completas (el resto pasa a `[salida borrada]`).
- `VAULT_HOME` cambia la carpeta del Dev Vault.
- En **Ajustes → Importar del Vault** se copia el endpoint detectado.
- `mute: false` deja los sonidos activados.
- Temas: `vercel` (predeterminado) y `midnight`.

## Autoedición segura

Si vas a modificar el propio cliente, usa los scripts incluidos:

```sh
./safe-edit.sh checkpoint           # commit del estado actual; imprime el hash base
# ...editar código...
./safe-edit.sh verify <hash-base>   # binario nuevo + go build + go test, revierte si falla
```

En Windows: `.\safe-edit.ps1 checkpoint` y `.\safe-edit.ps1 verify <hash-base>`.
El binario nuevo se escribe en `dist/nosecode-FECHA` (`.exe` en Windows), nunca
sobre el que está ejecutándose. Si fallan el build o los tests, el árbol vuelve
al hash base y se borran los archivos nuevos de `internal/` y `cmd/`.

El archivo se guarda con permisos `0600` porque contiene tu API key. No lo
subas a un repositorio.

## Diseño

El spec visual vive en `skills/nosecode-ui/SKILL.md` (paleta, layout, hitboxes,
sonidos y checklist de verificación).

## Licencia

MIT. Ver `LICENSE`. Los sonidos provienen de `@opencode-ai/ui` (MIT).
