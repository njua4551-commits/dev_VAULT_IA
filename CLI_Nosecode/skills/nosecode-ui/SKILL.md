---
name: nosecode-ui
description: Diseño y estilo de la TUI Nosecode (réplica del layout de opencode con tema Vercel, mouse, sonidos). Úsala al tocar la interfaz del proyecto Nosecode.
---

# Nosecode UI

TUI en Go + Bubble Tea / Lipgloss / Bubbles. Objetivo: mismo lenguaje visual
y misma disposición que opencode con tema `vercel`, sin copiar su código.

## Paleta (tema vercel, valores exactos)

| Token | Hex | Uso |
| --- | --- | --- |
| bg | `#000000` | fondo global |
| panel | `#1A1A1A` | status bar, superficies |
| element | `#292929` | chips, fila seleccionada, tab activa |
| border | `#1F1F1F` | separadores |
| borderActive | `#454545` | borde del prompt y modales |
| text | `#EDEDED` | texto principal |
| muted | `#878787` | etiquetas, metadatos |
| faint | `#5C5C5C` | pistas, texto secundario |
| primary | `#0070F3` | acento principal, marca, foco |
| secondary | `#52A8FF` | teclas y enlaces |
| accent | `#8E4EC6` | rol del asistente |
| success | `#46A758` | activo / on |
| warning | `#FFB224` | sin configurar / ask |
| danger | `#E5484D` | errores / deny |
| cyan | `#50E3C2` | detalles |

Tema alternativo `midnight`. `PaletteByName` debe resolver `""` → `vercel`.
No inventar colores fuera de `internal/ui/theme.go`.

## Layout (calcado de opencode)

Sin barra de pestañas: opencode no la tiene. Las sesiones viven en la barra
lateral y en la paleta (`/sessions`).

- **Home** (sin mensajes): logo en bloques 3 líneas (`█▀▄`, NOSE atenuado y
  CODE en blanco), prompt centrado (ancho 72 máx.), pista
  `shift+tab agentes   ctrl+p comandos` alineada a la derecha del prompt,
  `● Tip …` centrado, footer de 1 línea (cwd a la izquierda; `ctrl+p comandos`,
  `● proveedor` y versión a la derecha).
- **Prompt**: barra izquierda `┃` con el color del agente (Build = `secondary`,
  Plan = `accent`), fondo `element`, 2 líneas de texto, línea
  `Agente  modelo  proveedor` y borde inferior `╹▀▀▀`. Alto fijo `inputHeight`
  (6). Clic en el agente alterna Plan/Build; clic en modelo/proveedor abre ajustes.
- **Sesión**: cabecera en panel (`┃ # título`, contexto a la derecha si no hay
  barra lateral; 3 líneas si el alto ≥ 30), mensajes del usuario en panel con
  barra `┃` del agente, respuestas en texto plano con sangría de 3 y una línea
  `▣  nosecode · modelo` debajo, prompt abajo y barra lateral **derecha** de 38
  columnas (panel `panel`) si `UI.Sidebar` y ancho ≥ 100: título, contexto,
  proveedor, lista de sesiones (`+` nueva, `×` cierra la activa), y abajo cwd y
  versión.
- **Modales** (settings, paleta, renombrar, confirmar): panel `panel` sin
  borde, título en negrita y `esc` a la derecha, búsqueda, categorías en
  `accent` negrita con línea en blanco antes, fila seleccionada con fondo
  `primary` y texto blanco (`selectRow`), pie de ayuda. Nunca pantalla completa.
- Ventana menor de 44×14: mensaje centrado con la pista `ctrl+0` para el zoom.
- Todo cabe en 44×14, 80×24 y 120×40 sin desbordar ni cortar el borde inferior.

## Reglas de render

- Construir la pantalla como `[]uiLine`; cada línea registra `relHit` con
  `{x, w, action, arg}`. `View()` llama `register` y `normalizeScreen`, que
  pinta el fondo global `bg` en todo el lienzo.
- Paneles con fondo propio: usar `paint(color, texto)` (`paint.go`); vuelve a
  aplicar el fondo tras cada estilo anidado. Las filas seleccionadas se
  construyen con `selectRow`, nunca con estilos internos. Al meter la salida del
  textarea en un panel, pasarla por `stripBg`.
- `shiftLine` desplaza una línea (y sus hitboxes) a una columna.
- `pad`/`center`/`wrapText` son las únicas utilidades de ancho; usar
  `lipgloss.Width`, no `len`.
- Estilos solo desde `theme.go`: `textStyle`, `mutedStyle`, `faintStyle`,
  `primaryStyle`, `chipStyle`, `selectedStyle`, etc.
- Hover = `isHover(action, arg)`.

## Mouse

- Habilitado con `tea.WithMouseCellMotion()`.
- `MouseActionPress` + `MouseButtonLeft` → `activate(action, arg)`.
- `MouseActionMotion` → actualizar `hoverBox`.
- Rueda (`IsWheel`) → scroll del chat.
- Acciones obligatorias: `sessionSwitch`, `sessionClose` (arg = id),
  `sessionRename`, `mode`, `new`,
  `settingRow` (arg = índice filtrado), `settings`, `providerNew`,
  `providerDelete`, `providerActivate`, `providerImport`, `field`, `saveEdit`,
  `cancelEdit`, `paletteItem`, `theme`, `sound`, `focusInput`, `quit`,
  `slashItem` (arg = índice), `toolAllow`, `toolAllowSession`, `toolDeny`.
- Clic fuera de un modal cierra el modal. Toda zona clicable debe existir en el
  frame actual (los hitboxes se reconstruyen en cada render).

## Menú `/`, selección y agente

- `slash.go`: el menú aparece cuando la entrada empieza por `/` y aún no tiene
  espacios. Filtra por prefijo, se mueve con ↑/↓, completa con `tab`, ejecuta con
  `enter` y se oculta con `esc`. Se dibuja inmediatamente encima del prompt y no
  debe empujar el chat fuera de la pantalla.
- `selection.go`: arrastrar con el botón izquierdo selecciona por coordenadas;
  al soltar copia el texto plano al portapapeles. `ctrl+y` y `ctrl+c` copian la
  selección o la última respuesta. `esc` limpia la selección. El resaltado se
  aplica después de `normalizeScreen` y antes de registrar hitboxes.
- `agent.go`: el modal `screenTool` pide permiso por herramienta MCP. Acciones:
  `toolAllow`, `toolAllowSession`, `toolDeny`. Mientras conecta muestra
  `⏳ Conectando herramientas MCP…`; no permitir envíos duplicados.
- `ctrl+c` copia y nunca cierra la aplicación. Para salir: `ctrl+q` o `/quit`.
- `F8` abre la confirmación de apagado del Vault; no ejecutar `down` directo.

## Sonidos incluidos (MIT)

| Archivo | Cuándo |
| --- | --- |
| `bip-bop-01.mp3` | respuesta lista / guardado |
| `bip-bop-03.mp3` | pregunta al usuario |
| `staplebops-06.mp3` | permiso del agente |
| `nope-03.mp3` | error |
| `yup-01.mp3` | acción completada |

`UI.Mute` (false = con sonido). Reproducción nativa: MCI `winmm` en Windows,
`ffplay`/`mpg123`/`cvlc` en Linux, `afplay` en macOS; fallback campana.
Nunca bloquear el event loop: reproducir en goroutine.

## Verificación

```powershell
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
gofmt -w internal cmd
go vet ./...
go test ./...
go build -o dist\nosecode.exe ./cmd/nosecode
.\dist\nosecode.exe --demo=home
.\dist\nosecode.exe --demo=session
.\dist\nosecode.exe --demo=settings
.\dist\nosecode.exe --demo=palette
```

Tests mínimos: colores exactos de vercel, clic por coordenadas, render
44×14 / 80×24 / 100×30 / 120×40 (home, sesión, ajustes y paleta), prompt de
alto fijo, sesiones en la barra lateral y la paleta, migración de config a
versión 2.
