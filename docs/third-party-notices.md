# Third-party notices

VaultMind is MIT-licensed (see `LICENSE`). Go module dependencies and their
licenses are checked by `task check:license:source` and can be listed with
`task generate:license`; see [licenses.md](licenses.md).

This file covers code the binary embeds that is not itself a Go module,
which those tools can't see.

## PDFium (WebAssembly build)

`vaultmind import <url>` reads PDF text with PDFium. It's embedded as a
WebAssembly module by [go-pdfium](https://github.com/klippa-app/go-pdfium)
(MIT) and run in a sandbox by [wazero](https://github.com/tetratelabs/wazero)
(Apache-2.0).

- PDFium: <https://pdfium.googlesource.com/pdfium/>. go-pdfium's README states
  that PDFium comes under the Apache License 2.0. PDFium's own `LICENSE` file
  and the licenses of the libraries it bundles under `third_party/` (such as
  FreeType, OpenJPEG, libjpeg-turbo, lcms2 and zlib) are in its source tree.
- The embedded build is go-pdfium's `internal/pdfium_wasm/pdfium.wasm`, at the
  go-pdfium version pinned in `go.mod`.
