#!/usr/bin/env bash
# qtlib.sh — compile miqt's Qt5 C++ bindings once, in parallel, into a shared
# library (libmiqt-qt5.so), and make the Go build link against it.
#
# Why: miqt/qt has ~337 C++ files. `go build` compiles a package's C++ files
# almost one at a time, so the first build takes ages. This script uses every
# CPU core (make-style, incremental) and the Go build then only compiles Go.
#
#   tools/qtlib/qtlib.sh build     compile the .so (incremental, uses all cores)
#   tools/qtlib/qtlib.sh enable    build + point go.mod at the shared-lib copy
#   tools/qtlib/qtlib.sh disable   go back to the normal miqt (static C++)
#   tools/qtlib/qtlib.sh status    show what is active
#   tools/qtlib/qtlib.sh clean     delete the objects, the .so and the copy
#
# Env: JOBS (default: nproc), OPT (default: -O1), CXX (default: g++).
#
# The resulting meofishes binary needs libmiqt-qt5.so at run time; it finds
# it through the rpath recorded at link time (third_party/miqt-shared/lib).
# Elsewhere, run it with LD_LIBRARY_PATH pointing at a copy of the .so, or
# use the normal build for binaries you share.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
OUT="$ROOT/third_party/miqt-shared"
OBJ="$OUT/obj"
LIB="$OUT/lib"
JOBS=${JOBS:-$(nproc)}
OPT=${OPT:--O1}
CXX=${CXX:-g++}
MODPATH=github.com/mappu/miqt

cd "$ROOT"

miqt_version() { go list -m -f '{{.Version}}' "$MODPATH" 2>/dev/null; }

miqt_src() {
	# the pristine module from the Go module cache (download if needed)
	local v; v=$(miqt_version)
	go mod download "$MODPATH@$v" >/dev/null
	go list -m -f '{{.Dir}}' "$MODPATH@$v" 2>/dev/null || echo "$(go env GOMODCACHE)/$MODPATH@$v"
}

replaced() { go list -m -f '{{if .Replace}}yes{{end}}' "$MODPATH" 2>/dev/null | grep -q yes; }

cmd_build() {
	command -v pkg-config >/dev/null || { echo "pkg-config não encontrado" >&2; exit 1; }
	pkg-config --exists Qt5Widgets || { echo "Qt5 (headers) não encontrado: instale qt5-devel / qtbase5-dev" >&2; exit 1; }
	local SRC; SRC=$(cd "$(miqt_src)" && pwd)
	if [[ "$SRC" == "$OUT" ]]; then
		echo "go.mod já aponta para a cópia; usando o módulo original do cache" >&2
		SRC="$(go env GOMODCACHE)/$MODPATH@$(cat "$OUT/.version")"
	fi
	mkdir -p "$OBJ" "$LIB"
	local FLAGS; FLAGS="$(pkg-config --cflags Qt5Widgets) -std=c++11 -fPIC $OPT -w"
	echo "miqt: $SRC"
	echo "compilando $(ls "$SRC"/qt/*.cpp | wc -l) arquivos C++ com $JOBS processos ($OPT)…"
	export CXX FLAGS OBJ
	# incremental: only (re)compile files whose object is missing or older
	ls "$SRC"/qt/*.cpp | xargs -P "$JOBS" -I{} bash -c '
		src="{}"; o="$OBJ/$(basename "${src%.cpp}").o"
		if [[ ! -f "$o" || "$src" -nt "$o" ]]; then
			$CXX $FLAGS -c "$src" -o "$o.tmp" && mv "$o.tmp" "$o" && echo "  ✓ $(basename "$src")"
		fi'
	echo "linkando libmiqt-qt5.so…"
	# the Go callbacks (miqt_exec_callback_*) stay undefined here and are
	# resolved from the meofishes executable when it loads the library
	$CXX -shared -o "$LIB/libmiqt-qt5.so.tmp" "$OBJ"/*.o $(pkg-config --libs Qt5Widgets) -Wl,--allow-shlib-undefined
	mv "$LIB/libmiqt-qt5.so.tmp" "$LIB/libmiqt-qt5.so"
	miqt_version > "$OUT/.version"
	ls -lh "$LIB/libmiqt-qt5.so"
}

cmd_enable() {
	local SRC; SRC=$(cd "$(miqt_src)" && pwd)
	[[ "$SRC" == "$OUT" ]] && SRC="$(go env GOMODCACHE)/$MODPATH@$(cat "$OUT/.version")"
	cmd_build
	echo "criando a cópia do miqt sem os .cpp…"
	rm -rf "$OUT/mod.new"
	mkdir -p "$OUT/mod.new/qt" "$OUT/mod.new/libmiqt"
	cp "$SRC/go.mod" "$OUT/mod.new/"
	cp "$SRC"/libmiqt/* "$OUT/mod.new/libmiqt/"
	cp "$SRC"/qt/*.go "$SRC"/qt/*.h "$OUT/mod.new/qt/"
	cp -r "$SRC/qt/mainthread" "$OUT/mod.new/qt/"        # tiny, keeps its own C++
	rm -f "$OUT/mod.new/qt/cflags_windowsqtstatic.go"
	chmod -R u+w "$OUT/mod.new"
	cat > "$OUT/mod.new/qt/zz_meofishes_sharedlib.go" <<EOF
package qt

// Added by tools/qtlib/qtlib.sh: the C++ side of the bindings lives in
// libmiqt-qt5.so. --export-dynamic lets that library call back into Go.

/*
#cgo LDFLAGS: -L$LIB -lmiqt-qt5 -Wl,-rpath,$LIB -Wl,--export-dynamic
*/
import "C"
EOF
	rm -rf "$OUT/mod"
	mv "$OUT/mod.new" "$OUT/mod"
	go mod edit -replace "$MODPATH=./third_party/miqt-shared/mod"
	echo
	echo "pronto. Agora o build normal já usa a biblioteca:"
	echo '  CGO_CXXFLAGS="-O1 -w" go build -ldflags="-s -w" -o meofishes ./cmd/meofishes'
}

cmd_disable() {
	go mod edit -dropreplace "$MODPATH"
	echo "go.mod voltou para o miqt normal (o C++ volta a ser compilado pelo go build)."
}

cmd_status() {
	echo "miqt: $(miqt_version)"
	if replaced; then echo "modo: biblioteca dinâmica (third_party/miqt-shared)"; else echo "modo: normal"; fi
	if [[ -f "$LIB/libmiqt-qt5.so" ]]; then ls -lh "$LIB/libmiqt-qt5.so"; else echo "libmiqt-qt5.so: não compilada"; fi
	echo "objetos: $(ls "$OBJ"/*.o 2>/dev/null | wc -l)"
}

cmd_clean() {
	if replaced; then cmd_disable; fi
	chmod -R u+w "$OUT" 2>/dev/null || true
	rm -rf "$OUT"
	echo "removido $OUT"
}

case "${1:-}" in
	build) cmd_build ;;
	enable) cmd_enable ;;
	disable) cmd_disable ;;
	status) cmd_status ;;
	clean) cmd_clean ;;
	*) sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
