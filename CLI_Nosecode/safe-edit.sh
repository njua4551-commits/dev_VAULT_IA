#!/usr/bin/env sh
# Autoedicion segura de Nosecode.
#   ./safe-edit.sh checkpoint           guarda el estado actual en git y muestra el hash base
#   ./safe-edit.sh verify <hash-base>   compila a dist/nosecode-FECHA (sin tocar el binario en uso),
#                                       corre go build ./... y go test ./...; si algo falla,
#                                       revierte el arbol de trabajo al hash base.
set -eu

cd "$(dirname "$0")"

usage() {
	echo "Uso:"
	echo "  ./safe-edit.sh checkpoint"
	echo "  ./safe-edit.sh verify <hash-base>"
	echo
	echo "checkpoint  hace git commit de los cambios actuales y muestra el hash."
	echo "verify      compila el binario nuevo y valida build/tests; revierte si falla."
}

case "${1:-}" in
checkpoint)
	if [ -n "$(git status --porcelain)" ]; then
		git add -A
		git commit -m "Checkpoint antes de autoedicion ($(date '+%Y-%m-%d %H:%M:%S'))"
	fi
	git rev-parse HEAD
	;;
verify)
	BASE="${2:-}"
	if [ -z "$BASE" ]; then
		echo "Falta el hash base; usa: ./safe-edit.sh verify <hash-base>" >&2
		exit 2
	fi
	mkdir -p dist
	STAMP="$(date '+%Y%m%d-%H%M%S')"
	OUT="dist/nosecode-$STAMP"

	echo "Nosecode: compilando binario nuevo en $OUT (no se toca el que esta corriendo)..."
	if ! go build -trimpath -o "$OUT" ./cmd/nosecode; then
		echo "Fallo el build; revirtiendo a $BASE" >&2
		git reset --hard "$BASE"
		git clean -fd -- internal cmd
		exit 1
	fi

	echo "Nosecode: go build ./..."
	if ! go build ./...; then
		echo "Fallo go build; revirtiendo a $BASE" >&2
		git reset --hard "$BASE"
		git clean -fd -- internal cmd
		exit 1
	fi

	echo "Nosecode: go test ./..."
	if ! go test ./...; then
		echo "Fallo go test; revirtiendo a $BASE" >&2
		git reset --hard "$BASE"
		git clean -fd -- internal cmd
		exit 1
	fi

	echo "OK: binario nuevo en $OUT; build y tests pasan."
	;;
*)
	usage
	exit 2
	;;
esac
