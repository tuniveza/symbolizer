// imagine: brick's imagination neurons in Go, a drop-in for `node run.js`.
// It reads one JSON job on stdin and writes the JSON answer on stdout, exactly like run.js.
//
//	go build -o imagine ./cmd/imagine   (in brick/trainer/imagination/engine)
//	echo '{"list":true}' | ./imagine
//
// IMAGINE_CPUPROFILE=file writes a CPU profile of the job (for `go tool pprof`).
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"

	"imagination/engine"
)

func main() {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if p := os.Getenv("IMAGINE_CPUPROFILE"); p != "" {
		if f, err := os.Create(p); err == nil {
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}
	}
	out, err := engine.RunJob(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		pprof.StopCPUProfile()
		os.Exit(1)
	}
	os.Stdout.Write(out)
}
