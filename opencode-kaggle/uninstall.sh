#!/usr/bin/env bash
# Desinstala opencode-kaggle: lanzador, perfil, sesiones y caché.
set -euo pipefail

PREFIX="${PREFIX:-$HOME/.local}"
ROOT="$HOME/.local/share/opencode-kaggle"
CONF="$HOME/.config/opencode-kaggle"

if [ "${1:-}" != "--yes" ]; then
  echo "Se borrará:"
  echo "  $PREFIX/bin/opencode-kaggle"
  echo "  $CONF"
  echo "  $ROOT"
  echo "  $HOME/.local/state/opencode-kaggle"
  echo "  $HOME/.cache/opencode-kaggle"
  read -rp "¿Continuar? [s/N] " ok
  case "$ok" in
    s|S|si|Si|sí|Sí) ;;
    *) echo "cancelado"; exit 1 ;;
  esac
fi

if [ -L "$PREFIX/bin/opencode_n" ] && [ "$(readlink "$PREFIX/bin/opencode_n")" = opencode-kaggle ]; then
  rm -f "$PREFIX/bin/opencode_n"
fi
rm -f "$PREFIX/bin/opencode-kaggle"
rm -rf "$CONF" "$ROOT" "$HOME/.local/state/opencode-kaggle" "$HOME/.cache/opencode-kaggle"
echo "desinstalado"
