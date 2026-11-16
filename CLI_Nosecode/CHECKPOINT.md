# Checkpoint Nosecode 0.4.0 — 2026-11-02

Estado guardado en disco. El código compila, pasa `go vet` y `go test ./...`.
Binario instalado en `C:\Users\Enoc\AppData\Local\Nosecode\bin\nosecode.exe`.

## 0.9.0 — 2026-11-15 (imágenes, catálogo de IAs, avisos)

- **Imágenes del usuario** (`ui/attach.go`, `images/ansi.go`): `/img <ruta>`, `/img` (portapapeles: wl-paste, xclip o
  PowerShell), ctrl+v con imagen en el portapapeles, o arrastrar el archivo a la terminal (se detecta la ruta, con
  comillas, `\ ` o `file://`). Se reducen a JPEG ≤1024 px en `~/.nosecode/chat-images` (la sesión guarda solo la
  ruta, `Message.Images`). En el chat sale una **miniatura** de medios bloques ▀ a color verdadero (30×8) que se
  **amplía** con clic o ctrl+g (visor: ←/→ cambia, +/- tamaño, esc cierra). Al modelo viajan solo las imágenes de los
  2 últimos mensajes (ahorra contexto); máximo 4 por mensaje.
- **Catálogo de proveedores** (`config/catalog.go`, `ui/providers.go`): `/providers` o Ajustes → Proveedores →
  «Catálogo». Gemini, Claude, ChatGPT, DeepSeek, OpenRouter, Groq, Cerebras, GitHub Models, Mistral, xAI, NVIDIA,
  Ollama y LM Studio, con URL, modelo y contexto ya puestos y el enlace para conseguir la clave (varios con plan
  gratis). La clave se oculta (••••) al escribirla. Los modelos cambian: son editables.
  Para servicios en la nube (`config.IsCloudURL`) NO se envían `max_tokens` calculado ni `chat_template_kwargs`
  (solo para llama.cpp/Vault), porque esas APIs son estrictas.
- **Avisos**: `/up` y `/down` ya no escriben en el chat; el avance sale en la línea de actividad (`ui/notice.go`) y
  termina con «✓ IA encendida y lista» / «✓ IA apagada» (caduca a los 8 s). Errores en rojo. `/vault log` muestra la
  bitácora completa a pedido. `/urls` y `/vault status` siguen mostrando su salida en el chat.
- Pendiente de probar en vivo: miniaturas en tu terminal (necesita color verdadero), ctrl+v con captura real,
  y cada proveedor con una clave real (no se probó ninguno contra su API).

## 0.8.0 — 2026-11-15 (como un CLI de pago)

- **sudo con contraseña desde la TUI** (`tools/sudo.go`, `ui/sudo.go`): cualquier `sudo` pide primero aprobación
  (siempre, aunque bash esté en «permitir») y luego un modal con la contraseña enmascarada. Va a sudo por stdin
  (`sudo -S -p ''`), vive solo en memoria 5 min, se tapa con •••• en cualquier salida y nunca llega al modelo,
  a disco ni a debug.log. Si sudo la rechaza se olvida. Solo Linux/macOS; en Windows no aplica.
- **Diff con colores** en write_file/edit_file (`tools/diff.go`, `Result.Diff`, `ToolBlock.Diff`): `+N −M`,
  rojo/verde, 12 líneas (ctrl+o amplía). El diff es solo visual; el modelo no lo recibe.
- **Herramienta `todo`** (`tools/todo.go`, `ui/todos.go`): lista de tareas del agente, visible en la barra
  lateral («Tareas 2/5»). Permitida en Plan. Ayuda a que modelos pequeños no pierdan el hilo.
- **/export**: conversación a Markdown en `~/.nosecode/exports` (sin razonamiento, con claves ocultas).
- Prompt: regla de sudo actualizada (nunca pedir la contraseña en el chat) y regla de `todo`.
- Probado con un `sudo` falso en tests; falta probarlo con sudo real en CachyOS.

## 0.7.1 — 2026-11-15 (pensado para modelos más livianos)

- `tools/repair.go`: sinónimos de herramientas (`run`→bash, `read`→read_file…), prefijos (`functions.`),
  alias de argumentos (`cmd`→command, `file_path`→path…) y JSON roto (comas, comillas, ``` ) se corrigen solos.
- Guarda de bucles: la misma llamada 3 veces seguidas se bloquea con un mensaje que dice cambiar de enfoque.
- `/undo [list|force]` y `/redo` para write_file/edit_file (`tools/undo.go`); no cubre cambios hechos por bash.
- `ARQUITECTURA.md`: mapa de archivos y recetas para que un asistente no tenga que leer todo el código.
- Ideas siguientes: autocompactar al llegar al colchón, `/share`, modelo pequeño para títulos.

## 0.7.0 — 2026-11-15 (estilo opencode para un modelo local)

Hecho en bloques con pruebas; cada bloque es un commit.

- **A1** bloques de herramienta con estado, línea de actividad, vigilante de 45 s, cola.
- **A2** esc cancela de verdad, `timeout_s` de bash con «seguir/cancelar», `/status`, debug.log por herramienta.
- **B** delimitadores de web_fetch escapados; `NOSECODE_HOME` y debug.log tratados como sensibles.
- **C1** `permissions.bash_rules` (primera coincidencia gana; solo lista cerrada de solo lectura se auto-aprueba,
  nunca con metacaracteres de shell); Plan = agente de solo lectura (ctrl+e aprueba y ejecuta en Build);
  herramienta nativa `question` (modal 1-4 u otra respuesta). Código: `internal/tools/bashpolicy.go`,
  `internal/tools/question.go`, `internal/ui/plan.go`.
- **C2** `tools.compact_reserved` (8192): aviso de contexto, `max_tokens` = lo que queda de ventana;
  carga `NOSECODE.md` + `AGENTS.md` del proyecto + `~/.nosecode/AGENTS.md` (tope 16 KB);
  herramienta inexistente → error con la lista de disponibles; prompt de tareas acotadas. `internal/ui/instructions.go`.
- **Razonamiento** `reasoning_content` se muestra plegado (ctrl+t), `/thinking on|off|auto` envía
  `chat_template_kwargs.enable_thinking`; nunca se reenvía al modelo. `internal/ui/thinking.go`.
- **/init** el agente crea AGENTS.md. **/mcp add|remove** con confirmación del comando exacto (`internal/ui/mcpadd.go`).
- Sin hacer (ideas opencode): `/undo`, `/share`, subagentes, modelo pequeño para títulos.
- Pendiente de probar en vivo con el modelo de Kaggle: Plan sin escritura, modal `question`, bloque de razonamiento.

## Seguridad de herramientas 0.6.0 — 2026-11-12

- Punto 1: `permissions.read` (`ask` por defecto) y rutas sensibles canónicas
  (`filepath.Abs` + `EvalSymlinks`): `~/.dev-vault/`, `~/.nosecode/config.json`,
  `~/.ssh/`, `~/.gnupg/`, `~/.kaggle/`, `~/.aws/`, `~/.git-credentials`,
  gcloud, `.env`/`.env.*`, `*.pem` y `*.key`. Autorización por sesión y ruta
  exacta; las sensibles siempre se confirman. `grep` omite archivos sensibles
  salvo que la raíz aprobada sea sensible.
- Punto 2: `web_fetch` bloquea loopback, RFC1918, link-local, ULA, metadatos
  y nombres que resuelvan a esas IP. La IP se valida en `DialContext` y en cada
  redirección (máximo 5). `permissions.webfetch_local = "deny"` por defecto.
- Punto 3: webs y archivos son datos no confiables; regla en el prompt y
  delimitadores `<<<CONTENIDO_WEB_NO_CONFIABLE>>>` / `<<<FIN_CONTENIDO_WEB>>>`.
- Punto 4: el modal de `bash` muestra el comando completo; `debug.log` guarda
  aprobación/denegación/ejecución con secretos redactados.
- Punto 5: tests de `..`, symlink a `~/.ssh`, `.env`, loopback,
  169.254.169.254, redirección pública→privada, rangos IP y rutas normales.
- Verificación: `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...` y
  `GOOS=windows GOARCH=amd64 go build` pasan.
- Binario 0.6.0 instalado en Linux (`~/.local/bin/nosecode`) y generado
  `dist/nosecode-0.6.0.exe` para Windows.
- Commits: `6f6bb48`, `a52995e`, `007f47b`, `04076b9` (y el de docs/versión).

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
  ejecuciones de herramientas (sustituido en 0.5.0 por `tools.max_steps`
  configurable, con resumen automático y reanudación).
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
  de 1024 px, se aplanan sobre blanco y se convierten a JPEG calidad 82.
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

## Actualización 2026-11-10 — herramientas nativas, contexto y robustez

### Bloque A — herramientas

- Paquete `internal/tools`: `bash` (PowerShell o bash/sh, timeout 60 s, salida 8 KB),
  `read_file` con rango, `write_file`, `edit_file` único, `list_dir`, `grep`,
  `screenshot` (Wayland con `grim -`; X11 con `scrot`/`import`; Windows con PowerShell),
  `view_image`, `web_fetch` (HTML a texto, 20 KB) y `remember`.
- `screenshot`/`view_image` devuelven la imagen y el cliente la reduce a 1024 px
  antes de enviarla al modelo; las capturas se guardan en `~/.nosecode/tool-images`.
- Confirmación antes de `bash`, `write_file`, `edit_file` y herramientas de
  navegador/MCP, con «permitir en esta sesión».
- Las herramientas MCP ya no se envían por defecto. `browser_navigate`,
  `browser_click` y `browser_type` solo con `/tools browser on`; el resto de MCP
  solo con `/tools mcp on`. Las nativas están siempre activas.
- `/tools` lista las activas con tokens aproximados; `debug.log` registra al
  arrancar el número de herramientas y su tamaño.

### Bloque B — contexto y memoria

- `NOSECODE.md` (o `NOSECODE_MEMORY`) se carga en el mensaje de sistema y
  `remember` añade notas ahí.
- Microcompact: solo las últimas `tools.compact_keep` salidas de herramientas
  (6 por defecto) quedan completas; el resto pasa a `[salida borrada]`.
- `/compact` resume la conversación con el propio modelo, conserva el principio y
  el final, y continúa desde ese resumen.
- La ventana de contexto se lee de `/props` (`n_ctx`); si no está, se usa
  `context_window` del proveedor o 131072.
- La barra muestra tokens y porcentaje; `usage`/`timings` reales del servidor si
  existen. Al 80 % avisa con `/compact`.
- Un 400 con «exceeds the available context size» se muestra como contexto lleno
  con sugerencia de `/compact`, sin dejar el turno colgado.

### Bloque C — robustez

- MCP se conecta una sola vez al arrancar, en segundo plano y con timeout de 70 s.
  `debug.log` registra `connecting`, `error`, `timeout` y `ready`; la barra muestra
  «MCP conectando…» o el motivo del error.
- `safe-edit.sh` / `safe-edit.ps1`: checkpoint en git, binario nuevo en
  `dist/nosecode-FECHA` (sin tocar el que corre), `go build` + `go test` y
  reversión al hash base si falla, borrando también los archivos nuevos de
  `internal/` y `cmd/`.
- `TestIsChromium` normaliza separadores con `filepath` y acepta nombres Linux.
- `go build ./...`, `go vet ./...`, `go test ./...` y cross-compilación
  `GOOS=linux`/`GOOS=windows` pasan.

### Hecho el 2026-11-11

- **Nosecode 0.5.0**: el agente ya no muere al llegar a 12 pasos.
  - `tools.max_steps` configurable (30 por defecto, máximo 100) y
    `/tools max_steps N`; la barra muestra `paso N/M`.
  - Al agotar el límite se cierran las herramientas pendientes con resultados
    sintéticos, se pide un resumen final **sin herramientas** y el agente queda
    pausado conservando todo el historial (`a.msgs`). «continúa» reinicia solo
    el contador y reutiliza los resultados, sin repetir la investigación.
  - Prompt: agrupar llamadas independientes, usar `api.github.com`/`raw` o
    `git clone --depth 1` en vez del HTML de GitHub y resumir a los ~8 pasos.
  - `web_fetch` elimina `nav/header/footer/aside/form/svg/noscript/template`.
- Corregido el contador de GPU: `vault.py quota` emite `QUOTA_*` y Nosecode
  también acepta la línea legible antigua («GPU: usadas X h de Y h»).
  Verificado en vivo: `0.84 h usadas de 30 h, quedan 29.16 h`.
- El prompt del agente prohíbe expresamente decir que no puede ejecutar
  comandos, leer archivos o capturar la pantalla; debe usar `bash`,
  `read_file` o `screenshot`.
- Binario 0.5.0 compilado e instalado en Linux (`~/.local/bin/nosecode`) y
  generado `dist/nosecode-0.5.0.exe` para Windows.
- Los commits de los bloques A, B y C están subidos a `origin/main`.

## Pendiente

- Reinstalar el binario 0.6.0 en Windows y confirmar que el `nosecode` del PATH
  no es una versión antigua.
- Probar en la TUI un comando real del agente (`bash`) y confirmar que Qwen
  emite `tool_calls` tras la aprobación (el túnel de ngrok estaba offline en la
  última prueba).
- Probar el resumen automático al límite y «continúa» con el modelo real.
- Probar `/compact`, el contador de contexto y `screenshot` con la IA encendida.
- Refrescar `nosecode-public` y repetir el escaneo de secretos.
- Encender Kaggle con `/up` para la prueba en vivo y apagarlo con `/down` o `F8`.

Tests ya añadidos: selección por caracteres, recorte al panel, limpieza al copiar,
doble/triple clic, bloques de código, pestañas, herramientas nativas, permisos,
microcompact, `/compact`, cuota, parseo SSE de `usage`/`tool_calls`, límite de
pasos, resumen automático, reanudación con historial y limpieza de HTML.

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
