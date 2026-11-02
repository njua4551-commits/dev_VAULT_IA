# Dev Vault

Repositorio personal para **correr una IA potente gratis en Kaggle** y controlarla desde tu terminal: encender, apagar, recibir la URL sin entrar a la web y usarla por **web** o **API** con contrasena.

## Que hace

```
 vault.py (tu PC)  --API de Kaggle-->  Notebook en GPU T4 x2  --ngrok / Tailscale / Cloudflare-->  URLs (piden API key)
   up / down / chat / proxy            llama.cpp + Qwen3.8-27B                                     web UI + API OpenAI
```

- **Modelo:** Qwen3.8-27B con vision. Si no arranca, el notebook prueba solo: Qwen3.8 con menos contexto -> Qwen3.6-27B -> Qwen3.6 con menos contexto.
- **Tuneles (varios a la vez):** ngrok (dominio fijo gratis) + Tailscale Funnel (opcional, sin el cupo de 20 000 peticiones) + Cloudflare de respaldo (sin streaming). El cliente usa el primero que responde y **cambia solo** si uno falla o se queda sin cupo; `vault.py proxy` da lo mismo a opencode u otros clientes.
- **Contrasena y cifrado:** toda la API exige tu API key (48 caracteres aleatorios) y la web la pide antes de usarse; todo viaja por HTTPS (no es cifrado de extremo a extremo: el proveedor del tunel termina el TLS).
- **Ahorro de GPU:** `vault.py down`, apagado por inactividad (30 min) y tope de 8 h.

## Estructura

```
dev-vault/
├── client/
│   ├── vault.py                  # Cliente de terminal: init, up, down, chat, web, opencode...
│   └── requirements.txt
├── kaggle/notebooks/
│   └── run-qwen-llamacpp.ipynb   # Compila llama.cpp, descarga el modelo, tunel, apagado
├── configs/
│   ├── models.md                 # Modelos, respaldo y memoria
│   └── opencode.example.json
├── scripts/
│   └── chat.ps1                  # Cliente PowerShell (texto e imagenes)
├── instructions/
│   ├── client-guide.md           # EMPIEZA AQUI: guia paso a paso desde cero
│   └── kaggle-setup.md           # Ejecucion manual desde la web de Kaggle
├── .gitignore
└── README.md
```

## Inicio rapido (detalle en `instructions/client-guide.md`)

```
pip install kaggle
python -m kaggle auth login
python client/vault.py init --user TU_USUARIO_KAGGLE     # pide ngrok (y Tailscale, opcional)
python client/vault.py up                                 # enciende y espera la URL
python client/vault.py chat                               # o: web / key / urls / proxy / opencode
python client/vault.py down                               # apaga y deja de gastar GPU
```

## Estado y limitaciones

- Validado **offline** (cliente contra un Kaggle falso, notebook por funciones, cmake contra llama.cpp actual). **No ejecutado en Kaggle real**: la primera corrida es la prueba.
- Primera ejecucion: compilacion ~25 min + descarga ~19 GB; guarda el binario como Dataset para saltarla.
- ~30 h de GPU por semana, sesiones de hasta 12 h; no es un servidor 24/7.
- ngrok gratis: 1 GB de salida y 20 000 peticiones HTTP al mes (de ahi el cambio automatico a otros tuneles).
- **Tailscale dentro de Kaggle y SSE (streaming) en ngrok/Tailscale: sin probar.** Cloudflare Quick Tunnel no soporta SSE.
- El CLI de Kaggle no puede cancelar sesiones: `down` usa una senal por ntfy.sh y, si no llega, se detiene desde la web.
- Velocidad del 27B en 2xT4 sin medir. Cifras de benchmarks del fabricante, sin reproducciones independientes.
- Uso personal y de pruebas. Revisa los terminos de Kaggle antes de compartir el acceso.

## Seguridad

- No subas `~/.dev-vault/config.json` ni hagas publico el Dataset `vault-config`: contienen tu API key y tus tokens de ngrok/Tailscale.
- No pegues secretos en los prompts: el proveedor del tunel ve el trafico (HTTPS termina en sus servidores). Para mas privacidad: `init --ts-mode serve` (solo tu red Tailscale).
- Mantén el notebook **privado** y no compartas la API key.
- `ALLOW_COMMUNITY_BINARY` descarga un binario de terceros no auditado: dejalo en `False` salvo que entiendas el riesgo.
- Si conectas un agente, que pida confirmacion antes de ejecutar comandos o escribir archivos.
