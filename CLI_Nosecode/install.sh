#!/usr/bin/env sh
# Instala nosecode en ~/.local/bin y asegura el PATH.
set -eu

cd "$(dirname "$0")"
./build.sh

TARGET="$HOME/.local/bin"
mkdir -p "$TARGET"
cp dist/nosecode "$TARGET/nosecode"
chmod +x "$TARGET/nosecode"

case ":$PATH:" in
    *":$TARGET:"*) ;;
    *)
        for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
            if [ -f "$rc" ] && ! grep -q 'Nosecode' "$rc"; then
                printf '\n# Nosecode\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "$rc"
            fi
        done
        echo "Añadido $TARGET al PATH en .bashrc/.zshrc"
        ;;
esac

echo "Instalado: $TARGET/nosecode"
echo "Abre una terminal nueva (o ejecuta: export PATH=\"$TARGET:\$PATH\") y escribe: nosecode"
