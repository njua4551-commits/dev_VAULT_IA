# Guia: Qwen3.8-27B gratis en Kaggle, a mano desde la web de Kaggle

> Si quieres encender y apagar la IA **desde tu terminal** (recomendado), usa [`client-guide.md`](client-guide.md). Esta guia es para ejecutar el notebook manualmente.

Levanta `llama-server` (llama.cpp) con **Qwen3.8-27B** y vision en la GPU de Kaggle, por **web** y **API**.

> Estado: el notebook fue validado offline (sintaxis, logica de descarga, arranque de ngrok con respaldo de flags, apagado por senal e inactividad, cmake contra el llama.cpp actual). **No se ejecuto en una GPU real de Kaggle**: la primera corrida es la prueba. Incluye respaldo automatico de modelo.

## 1. Cuenta
1. Crea tu cuenta en kaggle.com y **verifica tu telefono** (Settings -> Phone Verification). Sin esto no hay Internet ni GPU.
2. Crea una cuenta gratis en **ngrok.com**: copia tu Authtoken y tu dominio gratuito (ej. `mi-nombre.ngrok-free.dev`).

## 2. Subir y configurar el notebook
1. Kaggle -> Create -> New Notebook -> File -> Import Notebook -> `kaggle/notebooks/run-qwen-llamacpp.ipynb`.
2. Settings: Accelerator **GPU T4 x2** (32 GB de VRAM; una P100 de 16 GB no puede cargar el modelo completo, que pesa ~19 GB), Internet **On**, Visibility **Private**.
3. Add-ons -> Secrets, crea y activa para este notebook:
   - `API_KEY`: clave larga (obligatoria: es tu contrasena).
   - `NGROK_AUTHTOKEN` y `NGROK_DOMAIN`: URL fija y streaming.
   - `TS_AUTHKEY` (y opcional `TS_HOSTNAME`): segundo tunel con Tailscale Funnel (ver `client-guide.md`, Paso 3b).
   - El notebook abre **a la vez** todos los tuneles con credenciales (+ Cloudflare de respaldo) e imprime una URL por tunel. A mano usa la que quieras; el cliente `vault.py` cambia solo.

## 3. Ejecutar
**Run All.** En orden: diagnostico -> `llama-server` (usa un binario guardado o lo compila, ~25 min) -> descarga del modelo (~19 GB, a la carpeta con mas espacio) -> arranque con respaldo automatico (Qwen3.8 64K -> Qwen3.8 32K -> Qwen3.6 64K -> Qwen3.6 32K) -> prueba de texto y vision -> tunel -> vigilancia.

La celda final mantiene la sesion y **la apaga sola** tras 30 min sin uso o a las 8 h (variables `IDLE_MINUTES` y `MAX_HOURS` en la celda 1). Tambien puedes detenerla a mano.

### Ahorrar los 25 minutos de compilacion
1. Tras la primera ejecucion, guarda una version (Save Version) y en **Output** crea un **New Dataset** privado con la carpeta `llama-bin`.
2. En los siguientes notebooks, agrega ese Dataset como Input. El notebook detecta `llama-server` en `/kaggle/input` y salta la compilacion.
3. El binario esta compilado para tu tipo de GPU (T4 = sm_75): si cambias de GPU, recompila.

## 4. Usar por web y por API
- **Web:** abre la URL impresa; la interfaz pide tu API key.
- **API:** base `https://TU-URL/v1`, modelo `Qwen3.8-27B` (o `Qwen3.6-27B` si se uso el respaldo), header `Authorization: Bearer TU_API_KEY`.

```powershell
.\scripts\chat.ps1 -Url "https://TU-URL" -Key "TU_API_KEY" -Prompt "Replica esta pantalla en HTML" -Image .\captura.png
```

### Con opencode
Usa `configs/opencode.example.json` (o `python client/vault.py opencode`) y registra la clave con `opencode auth login` (opcion *Other*, id `kaggle`). Con ngrok y dominio fijo la URL no cambia entre sesiones. **Imagenes en opencode:** segun un issue de su repositorio, opencode decide si acepta imagenes segun las capacidades declaradas del modelo; no pude confirmar como declararlas en un proveedor propio. Si no llegan, usa `vault.py chat` (`/img`) o tu propio cliente.

## 5. Ajustes (celda 1 del notebook)
| Variable | Efecto |
|---|---|
| `THINKING = False` | Mas rapido (recomendado en T4). `True` razona antes de responder: mejor calidad, mas lento. |
| `IDLE_MINUTES` / `MAX_HOURS` | Apagado por inactividad / tope de horas. |
| `ATTEMPTS`, `QUANTS` | Cadena de respaldo y cuantizaciones. |
| `TUNNEL` | `auto` (todos los que tengan credenciales) o una lista: `"ngrok,tailscale"`. `CLOUDFLARED_ALSO=false` (Secret) quita el respaldo. |
| `ALLOW_COMMUNITY_BINARY` | Solo si no hay `nvcc`: binario **de terceros** para T4, no auditado. Bajo tu riesgo. |

## Tuneles
Cloudflare Quick Tunnel **no soporta SSE** (streaming) segun su documentacion; es solo respaldo. ngrok gratis incluye un dominio fijo, con limites de 1 GB de salida y 20 000 peticiones HTTP al mes, por eso se anade Tailscale Funnel (URL fija, sin ese cupo). Tailscale dentro de Kaggle no esta probado. Los navegadores ven una pagina de aviso de ngrok una vez (no afecta a la API).

## Limites
- ~30 h de GPU por semana (varia), sesiones de hasta 12 h.
- No es un servidor 24/7: al terminar se pierde el modelo descargado y el tunel.
- Revisa los terminos de Kaggle antes de compartir el acceso con otras personas.
- La velocidad del 27B en 2xT4 no esta medida: es lo primero que conviene probar.

## Problemas comunes
| Sintoma | Causa probable |
|---|---|
| "No hay GPU" | Accelerator en None: ponlo en GPU T4 x2 y reinicia |
| "Sin internet" | Internet en Off o telefono sin verificar |
| "No hay nvcc" | La imagen no trae CUDA: usa un binario guardado en Dataset |
| Compilacion falla | Mira las ultimas lineas; prueba `LLAMA_REF` con un tag estable de llama.cpp |
| "no hay espacio suficiente" | Baja `QUANTS` a Q4_K_S |
| Pasa a Qwen3.6 | El intento con Qwen3.8 fallo: lee el log impreso (arquitectura, OOM...) |
| ngrok no arranca | Revisa `NGROK_AUTHTOKEN`/`NGROK_DOMAIN`; el notebook sigue con los otros tuneles |
| 401 en la API | Falta `Authorization: Bearer <API_KEY>` |
| Sin vision | El repo de respaldo no trae `mmproj`: el notebook lo avisa |
