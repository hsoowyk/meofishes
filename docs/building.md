# Building

Requirements: Go ≥ 1.26, a C++ compiler, `pkg-config`, Qt 5 headers
(`qt5-devel` on Void, `qtbase5-dev` on Debian/Ubuntu, `qt5-base` on Arch).

```sh
go build -ldflags="-s -w" -o meofishes ./cmd/meofishes
```

The first build compiles the Qt bindings (miqt, ~340 C++ files) and takes a
while and **a lot of RAM** (machines with less than ~6 GB may run out). Next
builds are fast.

## Faster: the Qt bindings as a shared library

`tools/qtlib/qtlib.sh` compiles the miqt C++ files once, in parallel, into
`third_party/miqt-shared/lib/libmiqt-qt5.so`, and points `go.mod` at a copy of
miqt that links against it:

```sh
JOBS=16 tools/qtlib/qtlib.sh enable
CGO_CXXFLAGS="-O1 -w" go build -ldflags="-s -w" -o meofishes ./cmd/meofishes
tools/qtlib/qtlib.sh status | disable | clean
```

A binary built this way needs the `.so` at run time (found through the rpath).
Use the normal build for binaries you share.

## Checking the interface without building Qt

`tools/uicheck` type-checks the packages that use Qt by parsing miqt's Go
files without compiling any C++. It needs ~200 MB of RAM and a few seconds:

```sh
go run ./tools/uicheck ./internal/ui ./cmd/meofishes
```

## Tests

```sh
CGO_ENABLED=0 go test ./internal/...            # offline tests
go test -tags live ./internal/plugin ./internal/lyrics ./internal/builtin/...   # hit real services
```

## CI

`.forgejo/workflows/build.yml` runs the offline tests and cross-compiles the
x86_64 binary on the arm64 runner (Debian multiarch Qt + x86_64 g++), with
the Go compiler's memory kept low. The binary is uploaded as an artifact.

## Translations

After changing interface strings run `go run ./tools/i18nkeys` (see
[i18n.md](i18n.md)).
