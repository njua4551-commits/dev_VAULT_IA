# AGENTS.md — Dev Vault IA

Guía breve para agentes que trabajen en este repositorio.

## Estructura

- `CLI_Nosecode/`: cliente de terminal en Go (TUI Bubble Tea). Habla con
  endpoints OpenAI-compatibles (llama-server del Vault) y tiene herramientas
  nativas: `bash`, archivos, `grep`, captura de pantalla, `web_fetch` y memoria.
- `dev-vault/`: scripts Python del Vault (Kaggle, túneles y `vault.py`).

## Estado actual (2026-11-11)

- Nosecode 0.5.0 compila y pasa `go build ./...`, `go vet ./...` y `go test ./...`
  en Linux, y cross-compila para Windows.
- Herramientas nativas activas por defecto; MCP y navegador solo con
  `/tools mcp on` y `/tools browser on`.
- Agente con límite configurable `tools.max_steps` (30 por defecto, máximo 100):
  al agotarlo pide un resumen sin herramientas, conserva `a.msgs` completo y
  queda pausado. «continúa» reanuda sin repetir la investigación. La barra
  muestra `paso N/M` y `/tools max_steps N` cambia el límite.
- Contexto: `/props` (`n_ctx`), `usage` real, aviso al 80 %, `/compact` y
  microcompact (últimas 6 salidas completas).
- Cuota de GPU: `vault.py quota` emite `QUOTA_*` y el cliente también entiende
  la línea legible antigua; se muestra en la barra y en la barra lateral.
- `web_fetch` elimina `nav/header/footer/aside/form/svg/noscript/template`
  antes de convertir el HTML a texto.
- Autoedición segura: `CLI_Nosecode/safe-edit.sh` y `safe-edit.ps1`.

## Pendiente

- Reinstalar el binario 0.5.0 en Windows (`install.ps1`) y confirmar que el
  `nosecode` del PATH no es una versión antigua.
- Probar en la TUI un comando real del agente (`bash`) y confirmar que Qwen
  emite `tool_calls` y que se ejecuta tras la aprobación (el túnel de ngrok
  estaba offline en la última prueba).
- Probar `/compact`, el contador de contexto y `screenshot` con la IA encendida.
- Refrescar la copia pública `nosecode-public` y repetir el escaneo de secretos.
- Investigar WordCraft con Nosecode 0.5.0 y confirmar que el resumen automático
  al llegar al límite funciona con el modelo real.

## Reglas para agentes

- No cambies `internal/mcp/mcp.go` ni la configuración de servidores MCP sin
  pedirlo: el objetivo es no enviar sus herramientas por defecto.
- Usa `path/filepath` para rutas; nunca separadores escritos a mano.
- Si modificas Nosecode, usa `safe-edit.sh checkpoint` y `safe-edit.sh verify`.
- No sobrescribas el binario que está en ejecución: compila a `dist/nosecode-FECHA`.
- Al tocar el bucle del agente, no borres `a.msgs` al pausar: el historial
  completo de herramientas es lo que permite continuar sin repetir trabajo.
- Antes de dar algo por terminado: `gofmt -w internal`, `go build ./...`,
  `go vet ./...` y `go test ./...`.
