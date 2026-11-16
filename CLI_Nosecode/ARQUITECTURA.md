# Mapa de Nosecode (para asistentes de IA)

Léelo ANTES de abrir código: dice qué archivo toca cada función, así no hay que revisar todo.
Detalle histórico por versión: `CHECKPOINT.md`. Reglas del proyecto: `../AGENTS.md`.

## Flujo de un mensaje
1. `ui/model.go` `submit()` → comandos `/…` van a `slash()`; el resto a `startAgent()` (`ui/agent.go`).
2. `beginAgent()` arma el historial (`ui/chat.go` `apiMessages`), el prompt (`systemPrompt()` +
   `loadInstructions()` en `ui/instructions.go`) y las herramientas (`apiTools()`).
3. `api/chat.go` `Stream()` hace SSE: texto, `reasoning_content`, tool_calls, usage.
4. `onAgentChunk()` pinta el stream. Al terminar: `tools.NormalizeCall` (`tools/repair.go`) repara
   nombre/argumentos y se ejecutan las llamadas una por una con `executeNextTool()`.
5. Antes de ejecutar: modo Plan (`planDenied`), `question`, regla de bash (`bashDecision`),
   guarda de bucles (`loopRepeated`), permisos de lectura (`readDecision`), aprobación (`callNeedsApproval`).
6. Resultado → `onToolDone()` → `appendToolResult()` → siguiente llamada o nueva vuelta al modelo.

## Dónde está cada cosa
| Tema | Archivo |
|---|---|
| Herramientas nativas (registro, esquemas) | `tools/tools.go`, `tools/fs.go`, `tools/exec.go`, `tools/web.go` |
| Permisos de lectura / rutas sensibles | `tools/access.go` |
| Reglas de bash por patrón | `tools/bashpolicy.go` (config: `permissions.bash_rules`) |
| `question` (esquema) / modal / Plan / bucles / `/undo` | `tools/question.go`, `ui/plan.go` |
| Reparar llamadas de modelos pequeños | `tools/repair.go` |
| Deshacer write/edit | `tools/undo.go` (pila en memoria, 50 entradas, ≤2 MB) |
| sudo (contraseña en TUI) | `tools/sudo.go`, `ui/sudo.go` (modal `screenSudo`, caché 5 min en memoria) |
| Diff de write/edit | `tools/diff.go` → `Result.Diff` → `ToolBlock.Diff` → `ui/toolblock.go` `diffRows` |
| Lista de tareas (`todo`) | `tools/todo.go`, `ui/todos.go` (panel lateral en `railLines`) |
| Imágenes del usuario: adjuntar, miniatura, visor | `ui/attach.go` (`thumb`, `imgViewState`), `images/ansi.go`, `ui/chat.go` `apiMessages` |
| Catálogo de proveedores | `config/catalog.go` (datos), `ui/providers.go` (modal + formulario), `ui/settings.go` |
| Avisos (/up, /down) | `ui/notice.go`, `ui/chat.go` `startVault`/`onVault`, `activityLineText` en `ui/view.go` |
| Exportar conversación | `ui/export.go` |
| Bloques de herramienta con estado | `ui/toolblock.go`, `session.ToolBlock` |
| Timeout suave de bash | `ui/bashtimeout.go`, `tools/exec.go` |
| Razonamiento (`/thinking`, ctrl+t) | `ui/thinking.go`, `api/chat.go` (`Chunk.Reasoning`) |
| Contexto: aviso, `max_tokens`, `/compact` | `ui/model.go` (`warnContext`), `ui/instructions.go` (`maxOutputTokens`), `ui/compact.go` |
| `/mcp add|remove|import` | `ui/mcpadd.go`, `ui/agent.go` (`mcpSlash`), `mcp/mcp.go` (no tocar) |
| Config y valores por defecto | `config/config.go` |
| Vista y modales | `ui/view.go` |
| Menú `/` y ayuda | `ui/slash.go`, `ui/model.go` (`helpLines`) |

## Cómo añadir…
- **Un comando `/x`**: `case` en `slash()` (model.go) + entrada en `ui/slash.go` + línea en la ayuda + test.
- **Una herramienta nativa**: `tools/*.go` (Tool + esquema + `register`), decidir si es de solo lectura
  (`planTools` en `ui/plan.go`), alias en `tools/repair.go`, línea en `agentSystem`.
- **Una opción de config**: struct + `Defaults()` + normalización en `config.go`, y test en `config_test.go`.
- **Un modal**: `screenX` en model.go, estado en `Model`, `keyX()` en `key()`, `xScreen()` y `case` en `View()`.

## Reglas que NO se deben romper
- `Model` es un valor (Bubble Tea): cada función devuelve el `Model` modificado; **`m = m.closeMCP()`**, no `m.closeMCP()`.
- Nada de lo que venga del modelo se ejecuta sin pasar por `bashDecision`/permisos; `allow` solo para la lista cerrada.
- El razonamiento (`Message.Think`) y los bloques `Message.Tool` nunca se reenvían al modelo.
- Para APIs en la nube no añadir parámetros propios de llama.cpp: usar `localMaxTokens()`/`localThinking()`.
- La contraseña de sudo solo existe en `Model.sudoPW` y en el `context` de una ejecución; nunca en mensajes, sesiones ni logs.
- No guardar claves en debug.log; `NOSECODE_HOME` y debug.log son rutas sensibles.

## Verificación estándar
```
gofmt -l . && go build ./... && go vet ./... && go test ./... && GOOS=windows go build ./...
```
Prueba en vivo pendiente (necesita Kaggle): Plan sin escritura, modal `question`, bloque de razonamiento,
`/undo` tras una edición real, bucle de llamadas repetidas.
