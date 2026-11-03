# Checkpoint Nosecode 0.4.0 — 2026-11-02

Estado guardado en disco. El código compila, pasa `go vet` y `go test ./...`.
Binario instalado en `C:\Users\Enoc\AppData\Local\Nosecode\bin\nosecode.exe`.

## Hecho

- Rediseño visual 0.4.0 (tema Vercel, sidebar derecha y pestañas de sesión).
- Fase 2 intacta: streaming, `/vault`, `/up`, `/down`, `/urls`, `/status`, `VAULT_SCRIPT`.
- Menú `/` estilo OpenCode: aparece al escribir `/`, filtra, flechas, tab y enter.
- Sesiones como pestañas de navegador sobre la cabecera (cambiar, cerrar, crear) sin quitar la barra lateral.
- Selección por caracteres limitada al panel de chat; copia limpia sin relleno, bordes ni sidebar.
- Doble clic selecciona palabra; triple clic selecciona el cuerpo del mensaje sin la cabecera `nosecode · modelo`.
- `/copy` copia la última respuesta y guarda bloques en `~/.nosecode/copies`; `/copy code [n]` y `/copy files`.
- `ctrl+y` copia la última respuesta; `ctrl+c` copia la selección y nunca cierra; `ctrl+q` sale.
- Apagado visible: `F8` abre confirmación y ejecuta `vault.py down`; también está en la paleta.
- Paleta: "Apagar la IA", "Herramientas MCP", "Conectar MCP".
- Ajustes: fila de permiso "Herramientas MCP" (ask/allow/deny).
- Ayuda, menú `/`, README y skill actualizados con `/copy`, pestañas, selección y atajos.
- Cliente MCP propio en `internal/mcp/mcp.go`: stdio local + HTTP remoto con headers, sin dependencias nuevas.
- Agente con herramientas en `internal/ui/agent.go`: bucle, aprobación por herramienta (una vez / sesión / denegar), imágenes guardadas en `~/.nosecode/tool-images`.
- `internal/api/chat.go` soporta `tools` y `tool_calls` en streaming.
- Importación MCP desde `~/.config/opencode/opencode.json`: 5 servidores detectados correctamente.
- Prueba MCP real: 4 servidores conectaron y expusieron 59 herramientas:
  - playwright 25
  - cisco-pt-mcp 11
  - cisco-packet-tracer-mcp 10
  - vmware-visual-mcp 13
  - `vmware-visual-mcp__list_windows` ejecutó correctamente.
- `guest-vmware` no respondió en `192.168.222.134:8765` (host apagado o IP/puerto distinto).
- Copia pública `nosecode-public/CLI_Nosecode` sincronizada con selección, pestañas, pruebas y documentación.
- Escaneo de la copia pública sin tokens, credenciales Kaggle ni rutas `C:\Users\...`; `go test ./...` pasa allí.

## Primera entrega de fiabilidad (tareas 1–4)

- Guardia de fallos: si una herramienta falla dos veces seguidas con el mismo error,
  se bloquea y responde «herramienta no disponible, no la reintentes»; deja de
  ofrecerse al modelo en los siguientes pasos.
- Límite de 12 pasos por mensaje del usuario, contando peticiones al modelo y
  ejecuciones de herramientas.
- Contexto cancelable por turno: `esc` corta el modelo en curso, la aprobación
  pendiente y la herramienta MCP en ejecución.
- Timeout de herramienta MCP: 60 s por defecto (`timeout` configurable en ms) y
  error claro «la herramienta X no respondio en Ns».
- Vigilancia de inactividad del modelo: 120 s sin tokens corta la espera con
  `ErrModelIdle` y un mensaje que sugiere `/up`; no salta a otro túnel tras el idle.
- Estado visible en la barra: `esperando al modelo… N s`, `recibiendo respuesta… N s`,
  `ejecutando <herramienta>… N s` y `procesando imagen… N s`.
- Navegador: `internal/browser` detecta el navegador predeterminado de Windows
  (Brave, Chrome, Edge, Opera o Vivaldi) y lo lanza con CDP y perfil aislado en
  `~/.nosecode/browser-profile` antes de una herramienta de Playwright. Si el
  predeterminado no es Chromium, devuelve un error claro; no usa Brave por defecto.
- Pruebas locales añadidas: guardia de fallos, límite de pasos, texto de actividad,
  cancelación por `esc`, modelo inactivo con `httptest`, timeout MCP con transporte
  simulado y detección de navegador. Sin red real, modelo ni navegador.
- `gofmt`, `go build ./...`, `go vet ./...` y `go test ./...` pasan.

## Segunda entrega (tareas 5–7)

- Imágenes de herramientas: se decodifican (PNG/JPEG/GIF), se reducen a un máximo
  de 1280 px, se aplanan sobre blanco y se convierten a JPEG calidad 82.
- La captura se guarda en `~/.nosecode/tool-images` y se envía al modelo como
  `image_url` con data URI, en un mensaje de usuario posterior al resultado de la
  herramienta.
- Solo se conservan las **dos últimas** imágenes en el historial; las anteriores
  mantienen su texto y un aviso de imagen omitida para ahorrar contexto.
- Estado MCP: el mensaje de conexión indica cuántos servidores se probarán; al
  terminar, el aviso muestra herramientas, servidores y errores, y `/mcp` conserva
  el detalle de los servidores fallidos.
- `~/.nosecode/debug.log`: registro rotativo (2 MB) con inicio/fin de conexión MCP,
  peticiones al modelo, herramientas con duración y error, cancelaciones y estado
  del navegador. No guarda claves, argumentos ni contenido del chat.
- Pruebas añadidas: reducción/JPEG de imágenes, poda a dos imágenes, serialización
  `image_url` y escritura de `debug.log`. `gofmt`, `go build`, `go vet` y
  `go test ./...` pasan.

## Pendiente

0. Binario actualizado con las tareas 1–7 y el indicador de cuota GPU de Kaggle
   (`GPU 26.8h` en la barra inferior y lateral). Reinicia Nosecode para usar esta
   versión; la cuota se refresca al arrancar, tras cada comando del Vault y cada 10 min.
1. Probar en la TUI un mensaje real del agente con aprobación y confirmar que Qwen emite `tool_calls`.
2. Refrescar `C:\Users\Enoc\Desktop\nosecode-public` y repetir escaneo de secretos.
3. Encender Kaggle con `/up` para la prueba en vivo y apagarlo después con `/down` o `F8`.

Tests ya añadidos: selección por caracteres, recorte al panel, limpieza al copiar,
doble/triple clic, bloques de código y pestañas. Sigue pendiente cubrir filtrado
del menú `/`, importación MCP y parseo SSE de `tool_calls`.

Estado 2026-11-02: `gofmt`, `go build ./...`, `go vet ./...` y `go test ./...`
pasan en el proyecto y en la copia pública. Binario 0.4.0 reinstalado en
`%LOCALAPPDATA%\Nosecode\bin\nosecode.exe`.

## Última actualización (tras primera prueba)

- `ctrl+c` ya NO cierra: copia la selección o la última respuesta. Para salir: `ctrl+q` o `/quit`.
- Conexión MCP con límites: 8 s por servidor al inicializar y 15 s para listar herramientas;
  antes el remoto caído podía bloquear hasta 300 s.
- Mensaje visible "⏳ Conectando herramientas MCP…" y bloqueo de envíos duplicados mientras conecta.
- Parseo de `tool_calls` más robusto: argumentos como string u objeto, nombres repetidos/fragmentados.
- Prompt del agente con instrucciones para listar ventanas, abrir apps (`win+r` → `calc` → `enter`)
  y verificar con `list_windows`; prohibido inventar ventanas.
- El config real `~/.nosecode/config.json` ya tiene los 5 servidores MCP importados y `mcp.enabled=true`.
- El notebook `dev-vault-qwen` está `COMPLETE`: hay que encenderlo con `/up` antes de probar el modelo.

## Limitaciones conocidas

- `go test -race ./...` no corre: falta cgo/gcc.
- Las imágenes MCP se guardan y se pasa la ruta como texto; aún no se envían como visión al modelo.
- El bucle del agente no se ha probado de extremo a extremo contra el modelo en vivo.
- `atotto/clipboard` ya era dependencia indirecta; no se tocó `go.mod`/`go.sum`.

## Comandos útiles

```powershell
cd C:\Users\Enoc\Desktop\CLI_Nosecode
go build ./...
go vet ./...
go test ./...
go build -trimpath -ldflags "-s -w" -o "$env:LOCALAPPDATA\Nosecode\bin\nosecode.exe" ./cmd/nosecode
```

Prueba MCP en vivo (opcional, gasta ~20 s y lee opencode.json):

```powershell
$env:NOSECODE_LIVE_MCP="1"
go test ./internal/mcp -run TestConnectAllLive -v -count=1
```

## Archivos clave

- `internal/ui/agent.go` — bucle del agente, aprobación, `/mcp`.
- `internal/mcp/mcp.go` — cliente MCP stdio/HTTP.
- `internal/ui/slash.go` — menú `/`.
- `internal/ui/selection.go` — selección y portapapeles.
- `internal/ui/model.go` — estado, teclas, F8, paleta.
- `internal/ui/view.go` — layout y modales.
- `internal/api/chat.go` — streaming con tool_calls.
- `internal/config/config.go` — MCP e importación.
- `internal/mcp/live_test.go` — prueba MCP real, omitida salvo `NOSECODE_LIVE_MCP=1`.
