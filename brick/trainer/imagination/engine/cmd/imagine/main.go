// imagine: brick's imagination neurons in Go, a drop-in for `node run.js`.
// It reads one JSON job on stdin and writes the JSON answer on stdout, exactly like run.js.
// --serve: stay running and take one job per stdin line, one answer per stdout line (like `run.js --serve`);
// the network is still read from its file for every job, so results are the same as one-shot runs.
//
//	go build -o imagine ./cmd/imagine   (in brick/trainer/imagination/engine; brick/make.sh does it)
//	echo '{"list":true}' | ./imagine
//
// IMAGINE_CPUPROFILE=file writes a CPU profile of the job (one-shot mode, for `go tool pprof`).
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"
	"unicode"

	"imagination/engine"
)

func main() {
	for _, a := range os.Args[1:] {
		if a == "--serve" {
			serve()
			return
		}
	}
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

func serve() {
	in := bufio.NewReaderSize(os.Stdin, 1<<20)
	out := bufio.NewWriter(os.Stdout)
	for {
		line, err := in.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimFunc(line, unicode.IsSpace) != "" {
			answer := runOne([]byte(line))
			out.Write(bytes.ReplaceAll(answer, []byte("\n"), []byte(" ")))
			out.WriteByte('\n')
			out.Flush()
		}
		if err != nil {
			return
		}
	}
}

// runOne answers one job; a failure becomes {"error": ...} like run.js --serve's catch.
func runOne(job []byte) (answer []byte) {
	defer func() {
		if r := recover(); r != nil {
			answer = errorJSON(fmt.Sprint(r))
		}
	}()
	out, err := engine.RunJob(job)
	if err != nil {
		return errorJSON(err.Error())
	}
	return out
}

func errorJSON(msg string) []byte {
	var b strings.Builder
	b.WriteString(`{"error":`)
	b.WriteString(fmt.Sprintf("%q", msg))
	b.WriteString("}")
	return []byte(b.String())
}
