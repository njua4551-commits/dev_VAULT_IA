#!/usr/bin/env bash
# Instala opencode-kaggle: copia el OpenCode que ya tienes, crea un perfil
# aislado y lo apunta a tu endpoint compatible con OpenAI servido en Kaggle.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BASE_URL="${KAGGLE_BASE_URL:-}"
MODEL="${KAGGLE_MODEL:-Qwen3.8-27B}"
API_KEY="${KAGGLE_API_KEY:-}"
PREFIX="${PREFIX:-$HOME/.local}"

usage() {
  cat <<EOF
Uso: ./install.sh --base-url URL [opciones]

Opciones:
  --base-url URL     endpoint compatible con OpenAI (obligatorio)
                     ejemplo: https://mi-tunel.ngrok-free.dev/v1
  --model NOMBRE     nombre del modelo (por defecto: Qwen3.8-27B)
  --api-key KEY      API key; si no se pasa, se pide por teclado
  --prefix DIR       dónde poner el lanzador (por defecto: ~/.local)
  -h, --help         esta ayuda

Variables equivalentes: KAGGLE_BASE_URL, KAGGLE_MODEL, KAGGLE_API_KEY, PREFIX.
El binario de OpenCode se toma de OPENCODE_BIN, de 'opencode' en el PATH o de
~/.opencode/bin/opencode.
EOF
}

die() { echo "error: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --base-url)   BASE_URL="${2:-}"; shift 2 ;;
    --base-url=*) BASE_URL="${1#*=}"; shift ;;
    --model)      MODEL="${2:-}"; shift 2 ;;
    --model=*)    MODEL="${1#*=}"; shift ;;
    --api-key)    API_KEY="${2:-}"; shift 2 ;;
    --api-key=*)  API_KEY="${1#*=}"; shift ;;
    --prefix)     PREFIX="${2:-}"; shift 2 ;;
    --prefix=*)   PREFIX="${1#*=}"; shift ;;
    -h|--help)    usage; exit 0 ;;
    *)            usage; die "opción desconocida: $1" ;;
  esac
done

if [ -z "$BASE_URL" ] && [ -t 0 ]; then
  read -rp "URL del endpoint compatible con OpenAI (https://.../v1): " BASE_URL
fi
[ -n "$BASE_URL" ] || die "falta --base-url (o KAGGLE_BASE_URL)"
case "$BASE_URL" in
  http://*|https://*) ;;
  *) die "la URL debe empezar por http:// o https://" ;;
esac
[ -n "$MODEL" ] || die "el modelo no puede estar vacío"

# Binario de OpenCode: no se descarga ni se redistribuye, se copia el que ya hay.
SRC="${OPENCODE_BIN:-}"
if [ -z "$SRC" ]; then
  if command -v opencode >/dev/null 2>&1; then
    SRC="$(command -v opencode)"
  elif [ -x "$HOME/.opencode/bin/opencode" ]; then
    SRC="$HOME/.opencode/bin/opencode"
  fi
fi
[ -n "$SRC" ] && [ -x "$SRC" ] || die "no encuentro OpenCode. Instálalo primero: https://opencode.ai/docs/  (o usa OPENCODE_BIN=/ruta/opencode)"

ROOT="$HOME/.local/share/opencode-kaggle"
CONF="$HOME/.config/opencode-kaggle"

mkdir -p "$ROOT/bin" "$CONF" "$PREFIX/bin"
install -m 755 "$SRC" "$ROOT/bin/opencode"
install -m 755 "$HERE/bin/opencode-kaggle" "$PREFIX/bin/opencode-kaggle"
install -m 644 "$HERE/config/cli.json" "$CONF/cli.json"

# Genera opencode.json con la URL y el modelo pedidos.
if [ -f "$CONF/opencode.json" ]; then
  cp "$CONF/opencode.json" "$CONF/opencode.json.bak"
  echo "copia de la configuración anterior: $CONF/opencode.json.bak"
fi
python3 - "$HERE/config/opencode.json" "$CONF/opencode.json" "$BASE_URL" "$MODEL" <<'PY'
import json, sys

template, dest, base_url, model = sys.argv[1:5]
with open(template, encoding="utf-8") as fh:
    cfg = json.load(fh)

cfg["model"] = f"kaggle/{model}"
provider = cfg["providers"]["kaggle"]
provider["settings"]["baseURL"] = base_url

placeholder = provider["models"].pop("__MODEL__")
placeholder["name"] = model
provider["models"][model] = placeholder

with open(dest, "w", encoding="utf-8") as fh:
    json.dump(cfg, fh, indent=2, ensure_ascii=False)
    fh.write("\n")
PY

# API key: nunca al repositorio, siempre a un archivo con permisos 600.
if [ -z "$API_KEY" ] && [ ! -s "$CONF/api-key" ] && [ -t 0 ]; then
  read -rsp "API key del endpoint (Enter para dejarla vacía): " API_KEY
  echo
fi
if [ -n "$API_KEY" ]; then
  umask 077
  printf '%s' "$API_KEY" > "$CONF/api-key"
  echo "API key guardada en $CONF/api-key"
elif [ ! -s "$CONF/api-key" ]; then
  umask 077
  : > "$CONF/api-key"
  echo "AVISO: no hay API key. Guárdala luego con: opencode-kaggle key"
fi

# Puerto propio para el servicio de fondo, para no chocar con otro OpenCode.
PORT="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
OPENCODE_CONFIG_DIR="$CONF" \
OPENCODE_DB="$ROOT/opencode.db" \
XDG_CONFIG_HOME="$CONF/xdg-config" \
XDG_DATA_HOME="$ROOT/xdg-data" \
XDG_STATE_HOME="$HOME/.local/state/opencode-kaggle" \
XDG_CACHE_HOME="$HOME/.cache/opencode-kaggle" \
  "$ROOT/bin/opencode" service set port "$PORT" >/dev/null 2>&1 || true

cat <<EOF

Listo.

  lanzador   $PREFIX/bin/opencode-kaggle
  config     $CONF/opencode.json
  API key    $CONF/api-key
  sesiones   $ROOT/opencode.db

Prueba:
  $PREFIX/bin/opencode-kaggle models
  $PREFIX/bin/opencode-kaggle run "Responde solo: OK"

Asegúrate de que $PREFIX/bin está en tu PATH.
EOF
