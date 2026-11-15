package engine

// The engine matches V8's arithmetic bit for bit. Where the Go compiler may fuse x*y+z into one FMA instruction
// (one rounding instead of two), results would drift from node's: on arm64, ppc64, s390x, riscv64 and loong64
// always, and on amd64 with GOAMD64=v3 or v4. Build for amd64 with GOAMD64=v1 (the default) or v2.

import (
	"runtime"
	"runtime/debug"
)

func init() {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "386" {
		panic("imagination engine: built for " + runtime.GOARCH + ", where Go fuses multiply-adds; results would not match node's. Build for amd64 (GOAMD64=v1 or v2).")
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "GOAMD64" && (s.Value == "v3" || s.Value == "v4") {
				panic("imagination engine: built with GOAMD64=" + s.Value + ", where Go fuses multiply-adds; results would not match node's. Build with GOAMD64=v1 or v2.")
			}
		}
	}
}
