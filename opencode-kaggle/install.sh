#!/usr/bin/env bash
# Instala opencode-kaggle en Linux o macOS.
#
# - Si ya tienes OpenCode, copia tu binario (perfil aislado).
# - Si no lo tienes, descarga el binario oficial para tu sistema.
# - Crea la configuración y el lanzador.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BASE_URL="${KAGGLE_BASE_URL:-}"
MODEL="${KAGGLE_MODEL:-Qwen3.8-27B}"
API_KEY="${KAGGLE_API_KEY:-}"
PREFIX="${PREFIX:-$HOME/.local}"
DOWNLOAD=0

usage() {
  cat <<EOF
Uso: ./install.sh --base-url URL [opciones]

Opciones:
  --base-url URL     endpoint compatible con OpenAI (obligatorio)
                     ejemplo: https://mi-tunel.ngrok-free.dev/v1
  --model NOMBRE     nombre del modelo (por defecto: Qwen3.8-27B)
  --api-key KEY      API key; si no se pasa, se pide por teclado
  --download         descarga OpenCode aunque ya esté instalado
  --prefix DIR       dónde poner el lanzador (por defecto: ~/.local)
  -h, --help         esta ayuda

Variables equivalentes: KAGGLE_BASE_URL, KAGGLE_MODEL, KAGGLE_API_KEY, PREFIX.
El binario de OpenCode se toma de OPENCODE_BIN, de 'opencode' en el PATH o de
~/.opencode/bin/opencode. Si no hay ninguno, se descarga el oficial.
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
    --download)   DOWNLOAD=1; shift ;;
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

ROOT="$HOME/.local/share/opencode-kaggle"
CONF="$HOME/.config/opencode-kaggle"
BIN="$ROOT/bin/opencode"
mkdir -p "$ROOT/bin" "$CONF" "$PREFIX/bin"

# ---------------------------------------------------------------------------
# 1. Binario de OpenCode
# ---------------------------------------------------------------------------
download_opencode() {
  local os arch ext url tmp pkg found
  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os" in
    Linux)  os=linux ;;
    Darwin) os=darwin ;;
    *) die "sistema no soportado: $os (usa install.ps1 en Windows)" ;;
  esac

  case "$arch" in
    x86_64|amd64)  arch=x64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die "arquitectura no soportada: $arch" ;;
  esac

  # Un x64 sin AVX2 necesita la variante baseline; Linux con musl, la musl.
  if [ "$os" = linux ] && [ "$arch" = x64 ]; then
    if ! grep -qw avx2 /proc/cpuinfo 2>/dev/null; then
      arch="x64-baseline"
    fi
    if ldd --version 2>&1 | grep -qi musl || ls /lib/ld-musl-* >/dev/null 2>&1; then
      arch="$arch-musl"
    fi
  fi

  if [ "$os" = linux ]; then ext=tar.gz; else ext=zip; fi
  url="https://opencode.ai/update/api/latest/cli/opencode/opencode-${os}-${arch}.${ext}"

  echo "Descargando OpenCode para $os-$arch..."
  echo "  $url"
  tmp="$(mktemp -d)"
  pkg="$tmp/opencode.$ext"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --progress-bar "$url" -o "$pkg"
  elif command -v wget >/dev/null 2>&1; then
    wget -q --show-progress -O "$pkg" "$url"
  else
    die "necesito curl o wget para descargar OpenCode"
  fi

  mkdir -p "$tmp/x"
  case "$ext" in
    tar.gz) tar -xzf "$pkg" -C "$tmp/x" ;;
    zip)
      command -v unzip >/dev/null 2>&1 || die "necesito unzip para extraer el binario"
      unzip -q "$pkg" -d "$tmp/x"
      ;;
  esac

  found="$(find "$tmp/x" -type f -name opencode | head -1)"
  [ -n "$found" ] || die "no encontré el binario dentro del archivo descargado"
  install -m 755 "$found" "$BIN"
  rm -rf "$tmp"
}

SRC="${OPENCODE_BIN:-}"
if [ "$DOWNLOAD" -eq 0 ]; then
  if [ -z "$SRC" ]; then
    if command -v opencode >/dev/null 2>&1; then
      SRC="$(command -v opencode)"
    elif [ -x "$HOME/.opencode/bin/opencode" ]; then
      SRC="$HOME/.opencode/bin/opencode"
    fi
  fi
  if [ -n "$SRC" ] && [ -x "$SRC" ]; then
    install -m 755 "$SRC" "$BIN"
    echo "OpenCode: $SRC ($("$SRC" --version 2>/dev/null | head -1))"
  else
    download_opencode
  fi
else
  download_opencode
fi

[ -x "$BIN" ] || die "no se pudo preparar el binario en $BIN"

# ---------------------------------------------------------------------------
# 2. Lanzador y configuración
# ---------------------------------------------------------------------------
install -m 755 "$HERE/bin/opencode-kaggle" "$PREFIX/bin/opencode-kaggle"
install -m 644 "$HERE/config/cli.json" "$CONF/cli.json"

if [ -f "$CONF/opencode.json" ]; then
  cp "$CONF/opencode.json" "$CONF/opencode.json.bak"
  echo "copia de la configuración anterior: $CONF/opencode.json.bak"
fi

python3 - "$HERE/config/opencode.json" "$CONF/opencode.json" "$BASE_URL" "$MODEL" "$CONF/api-key" <<'PY'
import json, sys

template, dest, base_url, model, key_file = sys.argv[1:6]
with open(template, encoding="utf-8") as fh:
    cfg = json.load(fh)

cfg["model"] = f"kaggle/{model}"
provider = cfg["providers"]["kaggle"]
provider["settings"]["baseURL"] = base_url
provider["settings"]["apiKey"] = "{file:" + key_file + "}"

placeholder = provider["models"].pop("__MODEL__")
placeholder["name"] = model
provider["models"][model] = placeholder

with open(dest, "w", encoding="utf-8") as fh:
    json.dump(cfg, fh, indent=2, ensure_ascii=False)
    fh.write("\n")
PY

# ---------------------------------------------------------------------------
# 3. API key
# ---------------------------------------------------------------------------
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
chmod 600 "$CONF/api-key"

# ---------------------------------------------------------------------------
# 4. Puerto propio del servicio de fondo
# ---------------------------------------------------------------------------
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
  "$BIN" service set port "$PORT" >/dev/null 2>&1 || true

echo
echo "Listo."
echo
echo "  lanzador   $PREFIX/bin/opencode-kaggle"
echo "  config     $CONF/opencode.json"
echo "  API key    $CONF/api-key"
echo "  sesiones   $ROOT/opencode.db"
echo
echo "Prueba:"
echo "  $PREFIX/bin/opencode-kaggle models"
echo "  $PREFIX/bin/opencode-kaggle run \"Responde solo: OK\""
echo
echo "Asegúrate de que $PREFIX/bin está en tu PATH."
