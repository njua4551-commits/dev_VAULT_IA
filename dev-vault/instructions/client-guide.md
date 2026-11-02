# Guia: controla tu IA de Kaggle desde tu terminal (todo gratis)

Con el cliente `client/vault.py` puedes **encender** la IA, **recibir la URL actualizada sin entrar a la web de Kaggle**, chatear, abrir la web con tu clave y **apagarla** para no gastar tus 30 h semanales de GPU. No necesitas pagar nada.

```
 Tu PC (vault.py)  --API de Kaggle-->  Notebook en GPU T4 x2  --ngrok-------->  URL fija  <--  tu PC / navegador
        |                                  llama-server + Qwen3.8-27B  --Tailscale--->  URL fija      (todas piden API key)
        |                                                              --Cloudflare-->  URL variable (respaldo)
        |-- "up":    kernels push (enciende) + kernels logs -f (lee la URL)
        |-- "down":  senal por ntfy.sh -> el notebook se apaga solo
```

## Lo que vas a necesitar (todo gratis)
1. **Python 3.10 o mayor** (python.org -> Downloads; en Windows marca *Add Python to PATH*).
2. **Cuenta de Kaggle con telefono verificado** (kaggle.com -> Settings -> Phone Verification).
3. **Cuenta de ngrok** (ngrok.com, plan gratuito). Da **un dominio fijo gratis**, asi la URL no cambia nunca.
4. **Cuenta de Tailscale** (tailscale.com, plan Personal gratuito) - *opcional pero recomendada*: es un segundo tunel con URL fija **sin el limite de 20 000 peticiones de ngrok**. Ver Paso 3b.

> Por que ngrok y no Cloudflare o localtunnel: el Quick Tunnel de Cloudflare **no soporta SSE** (el streaming que usa el chat y los agentes, segun su documentacion); el tunel con nombre de Cloudflare exige un dominio propio (de pago); localtunnel depende de un servidor comunitario con caidas ocasionales y el subdominio no esta garantizado. ngrok gratis tiene limites (1 GB de salida y 20 000 peticiones HTTP al mes): un agente que lee y escribe archivos las gasta rapido.

**Por eso abrimos varios tuneles a la vez** y el cliente cambia solo al que funcione:

| Tunel | Cuenta | URL | Streaming | Limites gratis | Prioridad |
|---|---|---|---|---|---|
| ngrok | si | fija | si* | 20 000 peticiones y 1 GB al mes | 1 |
| Tailscale Funnel | si | fija (`*.ts.net`) | por confirmar* | sin cupo de peticiones publicado; Funnel es beta y tiene limites de ancho de banda no documentados con precision | 2 |
| Cloudflare Quick Tunnel | no | cambia en cada sesion | **no** | sin garantia de disponibilidad | 3 (respaldo) |

\* No pude confirmar en la documentacion que ngrok ni Tailscale pasen SSE sin problema; la primera conversacion con streaming lo revela. Si te quedas sin tunel con streaming, el cliente pide respuestas completas (sin streaming) a Cloudflare.

## Paso 1. Instalar el paquete de Kaggle
Abre una terminal (PowerShell en Windows) y ejecuta:
```
pip install kaggle
```

## Paso 2. Iniciar sesion en Kaggle desde la terminal
```
python -m kaggle auth login
```
Se abre el navegador para autorizar. (Alternativa: crea un token en kaggle.com/settings/api y sigue las instrucciones que muestra el comando.)

## Paso 3. Datos de ngrok
1. Crea tu cuenta en ngrok.com e inicia sesion en su dashboard.
2. Copia tu **Authtoken** (seccion "Your Authtoken" / Getting Started).
3. Busca tu **dominio gratuito** en la seccion *Domains* (algo como `mi-nombre.ngrok-free.dev`). El plan gratuito incluye uno.

## Paso 3b (opcional, recomendado). Tailscale como segundo tunel
1. Crea tu cuenta en tailscale.com (puedes entrar con Google o GitHub).
2. En el panel de administracion -> **DNS**: activa **MagicDNS** y **HTTPS Certificates**.
3. En **Access controls**: Funnel necesita el permiso `funnel`. Si en la primera ejecucion el notebook imprime un enlace para habilitar Funnel, abrelo y acepta (se hace una sola vez).
4. **Settings -> Keys -> Generate auth key**: marca *Reusable* y *Ephemeral* (la maquina de Kaggle desaparece sola de tu lista al terminar). Copia la clave `tskey-auth-...` (caduca como maximo en 90 dias; entonces genera otra y repite `init`).
5. La pasas en el siguiente paso con `--ts-authkey`.

> Honestidad: Tailscale en un contenedor de Kaggle (sin `/dev/net/tun`, modo "userspace") y Funnel desde ahi **no los pude probar**. Si falla, el notebook lo dice y sigue con los otros tuneles: no rompe nada.

## Paso 4. Configurar (una sola vez)
Desde la carpeta del repo:
```
python client/vault.py init --user TU_USUARIO_KAGGLE
```
Te pedira el authtoken y el dominio de ngrok (`--ngrok-token`, `--ngrok-domain`) y, si quieres, la clave de Tailscale (`--ts-authkey`, y opcionalmente `--ts-hostname`). Con `--no-cloudflared` no se abre el tunel de respaldo. Esto:
- genera tu **API key** (la contrasena de tu IA) y un tema secreto de apagado,
- guarda todo en `~/.dev-vault/config.json` (solo en tu PC),
- sube un **Dataset privado** `vault-config` a tu Kaggle con esos datos para que el notebook los lea.

> Nunca hagas publico ese Dataset: contiene tu API key y tu token de ngrok.

## Paso 5. Encender
```
python client/vault.py up
```
Sube el notebook, lo ejecuta en **GPU T4 x2** con Internet y **espera hasta que el modelo este listo**, mostrando las fases. Al terminar imprime la URL principal y los tuneles de respaldo. La primera vez tarda mucho (compilacion ~25 min + descarga ~19 GB); las siguientes veces, mucho menos si guardaste el binario como Dataset (ver `kaggle-setup.md`).

### Ahorra ~27 min de GPU en cada encendido (hazlo una vez)
La primera vez el notebook compila `llama-server` (en tu prueba real tardo **27.3 min**, que cuentan de tus 30 h). Guardalo una vez:
1. Con la IA lista, apagala con `python client/vault.py down` y **espera a que termine sola** (estado COMPLETE). Si cancelas la sesion a mano desde la web, Kaggle no guarda los archivos.
2. `python client/vault.py save-binary` descarga el binario compilado y lo sube como Dataset privado `llama-bin-t4`.
3. Espera ~2 min. Los siguientes `up` lo usan y saltan la compilacion.
(No probado en Kaggle real.)

## Paso 6. Usarla
| Comando | Que hace |
|---|---|
| `python client/vault.py chat` | Chat en la terminal con streaming. `/img ruta.png` adjunta una imagen, `/exit` sale. |
| `python client/vault.py web` | Abre la web UI y te muestra la API key para pegarla. |
| `python client/vault.py key` | Muestra tu API key. |
| `python client/vault.py url` | Muestra la URL actual (comprueba que responde; si no, cambia a otro tunel). |
| `python client/vault.py save-binary` | Guarda el llama-server compilado como Dataset para no recompilar (ver arriba). |
| `python client/vault.py urls` | Lista todos los tuneles y cuales estan vivos. |
| `python client/vault.py proxy` | Proxy local `http://127.0.0.1:8787` que pone tu API key y **cambia de tunel solo** si uno falla o se queda sin cupo. |
| `python client/vault.py status --quota` | Estado del notebook y horas de GPU usadas (la cuota es experimental). |
| `python client/vault.py logs -f` | Logs en vivo del notebook. |
| `python client/vault.py opencode` | Escribe `opencode.json` con la URL actual (ver `configs/opencode.example.json`). Con `--proxy` apunta al proxy local para tener cambio automatico de tunel (deja `vault.py proxy` corriendo en otra terminal). |

**Cambio automatico:** `chat` y `proxy` prueban primero el tunel actual; si responde con error de conexion o de cupo (HTTP 402, 403, 429, 500, 502, 503, 504) pasan al siguiente. Un 401 (clave mala) no cambia de tunel.

## Paso 7. Apagar
```
python client/vault.py down
```
Envia la senal de apagado; el notebook cierra el servidor y el tunel, termina, y la sesion deja de gastar GPU. El comando espera hasta 5 minutos y confirma.

**Red de seguridad:** aunque te olvides, el notebook se apaga solo si pasan **30 min sin uso** o **8 horas** (cambialo con `init --idle-minutes 15 --max-hours 4`, luego `up --sync-config`).

## Contrasena y cifrado: quien puede usarla
**Contrasena (API key).** `init` genera una clave aleatoria de 48 caracteres (`secrets.token_urlsafe(36)`).
- **API:** todas las rutas exigen `Authorization: Bearer TU_API_KEY` (lo comprobe en el codigo de llama-server: solo `/health` y los archivos de la interfaz son publicos). Sin clave responde 401.
- **Web:** la interfaz carga, pero pide la API key antes de poder usar nada.
- **Cliente / proxy:** `vault.py` la envia en cada peticion; la guarda en `~/.dev-vault/config.json` (permisos 600 en Linux/Mac).
- Cambiarla: `python client/vault.py init --reset-key` y luego `up --sync-config`.
- Una clave de 48 caracteres aleatorios no se adivina por fuerza bruta. Lo que no puedo asegurar es que llama-server limite los intentos fallidos; por eso la clave larga es la proteccion real: no la compartas, no la subas a GitHub y no hagas publico el Dataset `vault-config`.

**Cifrado.** Todos los tuneles usan **HTTPS (TLS)**: lo que va entre tu PC y el tunel va cifrado. Limite real: **no es cifrado de extremo a extremo**. ngrok, Tailscale Funnel y Cloudflare terminan el HTTPS en sus servidores, asi que tecnicamente el proveedor del tunel podria ver tus prompts y respuestas (igual que pasa con cualquier servicio en la nube). Si eso te preocupa:
- No pegues secretos en los prompts (claves, datos de clientes).
- Opcion mas privada: `init --ts-mode serve` expone la IA **solo dentro de tu red Tailscale** (cifrada con WireGuard, sin URL publica). Exige instalar Tailscale en tu PC; no la probe en Kaggle.
- El apagado remoto usa ntfy.sh con un tema aleatorio y una firma HMAC de tu clave: sin tu clave nadie puede apagar tu sesion, y la senal no revela la clave.

## Que esta probado y que no
- **Probado offline** con un Kaggle falso y servidores locales: init, encendido, lectura de las URLs desde los logs, no repetir el push, chat con streaming, **eleccion del primer tunel vivo, cambio automatico en chat y proxy (429/503/caida), respuesta sin streaming para Cloudflare, 401 sin cambiar de tunel**, `opencode.json` y apagado. Tambien la logica del notebook (varios tuneles en paralelo, URL principal, tunel caido, apagado por senal e inactividad).
- **No probado contra Kaggle real** (tu primera corrida lo es): el formato exacto de las respuestas de `push`/`status`, que los `print` del notebook aparezcan en `kernels logs -f`, el tiempo que tarda Kaggle en procesar el Dataset, que el Dataset de configuracion se adjunte bien al lanzar por API, y que `ngrok http --url=...` acepte tu dominio. **Tailscale** (instalar y arrancar `tailscaled` en modo userspace dentro de Kaggle, y que Funnel publique el puerto) es lo menos seguro de todo: puede fallar y el notebook seguira con los demas tuneles. Si algo falla, el mensaje te dice donde; pasamelo y lo ajusto.
- El Kaggle CLI no tiene comando para cancelar una sesion; por eso `down` usa una senal por ntfy.sh (servicio gratuito, sin cuenta). Si no llegara, detén la sesion desde la web de Kaggle.

## Problemas comunes
| Sintoma | Que hacer |
|---|---|
| `Kaggle no tiene tus credenciales` | `python -m kaggle auth login` |
| `Kaggle rechazo el push` | El Dataset aun se procesa (espera 1-2 min) o falta verificar el telefono |
| `up` no muestra la URL | `python client/vault.py logs -f` para ver donde se quedo; mira tambien la pagina del notebook en Kaggle |
| La web pide clave | Es lo esperado: `python client/vault.py key` |
| Chat entrecortado | Estas usando Cloudflare (sin streaming). Configura ngrok y/o Tailscale y `init` de nuevo |
| `[ngrok] fallo (HTTP ...)` | Posible cupo agotado (no pude confirmar con que codigo responde ngrok): el cliente ya paso al siguiente tunel |
| Tailscale `no dio URL` | Habilita Funnel (enlace en la salida) y MagicDNS/HTTPS en el panel; revisa que la auth key no haya caducado |
| `down` no apaga | Detén la sesion en la web de Kaggle; revisa que el notebook estuviera ya en ejecucion (no en cola) |
| 401 Invalid API Key | La clave del cliente no coincide con la del Dataset: `up --sync-config` |

## Siguiente paso: tu propio agente tipo opencode
`vault.py chat` ya hace la parte dificil (conexion, clave, streaming, imagenes). Para convertirlo en agente:
1. Define herramientas (leer archivo, escribir archivo, ejecutar comando, buscar texto) y envialas en el campo `tools` de `/v1/chat/completions` (el servidor corre con `--jinja`, que habilita el tool calling).
2. Bucle: si la respuesta trae `tool_calls`, ejecuta la herramienta **en tu PC**, agrega el resultado como mensaje `tool` y vuelve a llamar, hasta que el modelo responda texto.
3. Pide confirmacion antes de escribir archivos o ejecutar comandos.
4. Envia capturas como `image_url` (base64), tal como hace `/img`.

Si prefieres no escribirlo, `vault.py opencode` deja listo el proveedor para opencode (las imagenes pueden no pasar en opencode; ver `kaggle-setup.md`).
