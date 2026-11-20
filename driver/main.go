// drive runs repro binaries at a 40x24 pty read by a terminal emulator (charmbracelet/x/vt), and counts
// whether the menu is still on screen after ctrl-c.
//
//	drive -n 100 -load none|inproc|ext -out DIR BIN...
//
// Runs interleave the binaries (run 1 of each, then run 2 of each, ...) so load drifts hit all alike. Each run
// waits for the menu, sends ctrl-c, waits for the exit, and records: the final screen (kept: all four menu lines
// shown; erased: none; partial-KofN: some, e.g. the last line cleared), the bytes the program wrote after the key, whether those held an erase (CSI J or
// CSI K), the exit code, and the minimal repro's view-before-size trace. One CSV row per run goes to
// DIR/runs.csv, each run's pty log to DIR/logs, and a summary with Wilson 95% intervals to stdout.
//
// Load: inproc is 3 busy goroutines per CPU inside this driver; ext is 10 separate busy processes, each
// with a busy goroutine per CPU; none is no added load.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var eraseRE = regexp.MustCompile(`\x1b\[[0-9;]*[JK]`)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "spin" {
		for range runtime.NumCPU() - 1 {
			go func() {
				for {
				}
			}()
		}
		for {
		}
	}
	n := flag.Int("n", 100, "runs per binary")
	load := flag.String("load", "inproc", "none, inproc or ext")
	out := flag.String("out", "out", "output folder")
	flag.Parse()
	bins := flag.Args()
	must(os.MkdirAll(filepath.Join(*out, "logs"), 0o755))
	csv, err := os.Create(filepath.Join(*out, "runs.csv"))
	must(err)
	defer csv.Close()
	fmt.Fprintln(csv, "goos,load,bin,run,result,exit,bytes_after_key,erase_after_key,view_before_size,ms,final_screen")

	var stop atomic.Bool
	var spinners []*exec.Cmd
	switch *load {
	case "inproc":
		for range runtime.NumCPU() * 3 {
			go func() {
				x := 0
				for !stop.Load() {
					x++
				}
				_ = x
			}()
		}
	case "ext":
		self, err := os.Executable()
		must(err)
		for range 10 {
			c := exec.Command(self, "spin")
			must(c.Start())
			spinners = append(spinners, c)
		}
	case "none":
	default:
		must(fmt.Errorf("unknown load %q", *load))
	}
	defer func() {
		stop.Store(true)
		for _, c := range spinners {
			_ = c.Process.Kill()
			_ = c.Wait()
		}
	}()
	time.Sleep(2 * time.Second) // let the load reach every core before the first run

	fmt.Printf("goos=%s goarch=%s cpus=%d go=%s load=%s n=%d start=%s\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version(), *load, *n, time.Now().UTC().Format(time.RFC3339))
	type tally struct {
		kept, erased, other, eraseSeq, vbs, traced int
		partial                                    map[string]int
	}
	t := map[string]*tally{}
	for _, b := range bins {
		t[b] = &tally{partial: map[string]int{}}
	}
	for i := 1; i <= *n; i++ {
		for _, b := range bins {
			name := strings.TrimSuffix(filepath.Base(b), ".exe")
			r := run(b, filepath.Join(*out, "logs", fmt.Sprintf("%s-%03d", name, i)))
			fmt.Fprintf(csv, "%s,%s,%s,%d,%s,%d,%d,%v,%s,%d,%q\n", runtime.GOOS, *load, name, i, r.result, r.exit, r.after, r.erase, r.vbs, r.ms, r.screen)
			x := t[b]
			switch r.result {
			case "kept":
				x.kept++
			case "erased":
				x.erased++
			default:
				x.other++
				x.partial[r.result]++
			}
			if r.erase {
				x.eraseSeq++
			}
			if r.vbs != "" {
				x.traced++
				if r.vbs == "true" {
					x.vbs++
				}
			}
		}
	}
	fmt.Printf("end=%s\n", time.Now().UTC().Format(time.RFC3339))
	for _, b := range bins {
		x := t[b]
		lo, hi := wilson(x.erased, *n)
		fmt.Printf("%-16s load=%-6s kept %3d  erased %3d (95%% CI %5.1f%%-%5.1f%%)  other %3d %v  erase-seq-after-key %3d", strings.TrimSuffix(filepath.Base(b), ".exe"), *load, x.kept, x.erased, 100*lo, 100*hi, x.other, x.partial, x.eraseSeq)
		if x.traced > 0 {
			fmt.Printf("  first-view-before-size %d/%d", x.vbs, x.traced)
		}
		fmt.Println()
	}
}

type result struct {
	result, vbs, screen string
	exit, after int
	erase       bool
	ms          int64
}

func run(bin, log string) result {
	begin := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	trace := log + ".trace"
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(), "BT_TRACE="+trace)
	p, err := start(ctx, cmd, 40, 24, log+".pty")
	if err != nil {
		return result{result: "start-error"}
	}
	defer p.close()
	if !p.waitFor(20*time.Second, func(s string) bool { return strings.Contains(s, "Ship") }) {
		return result{result: "no-menu"}
	}
	if err := p.write([]byte("\x03")); err != nil {
		return result{result: "send-error"}
	}
	r := result{exit: p.wait()}
	s := p.screen()
	r.ms = time.Since(begin).Milliseconds()
	r.screen = s
	shown := 0
	for _, line := range []string{"What now?", "Build", "Test", "Ship"} {
		if strings.Contains(s, line) {
			shown++
		}
	}
	switch shown {
	case 4:
		r.result = "kept"
	case 0:
		r.result = "erased"
	default:
		r.result = fmt.Sprintf("partial-%dof4", shown)
	}
	p.close()
	r.after, r.erase = afterKey(log + ".pty")
	if b, err := os.ReadFile(trace); err == nil {
		r.vbs = strings.TrimPrefix(strings.TrimSpace(string(b)), "view-before-size=")
	}
	return r
}

// afterKey reads a pty log ("time dir quoted-bytes" per line) and returns how many bytes the program wrote
// after ctrl-c, and whether they held an erase.
func afterKey(path string) (int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return -1, false
	}
	defer f.Close()
	var after strings.Builder
	seen := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), " ", 3)
		if len(parts) < 3 {
			continue
		}
		b, err := strconv.Unquote(parts[2])
		if err != nil {
			continue
		}
		if parts[1] == "write" && strings.Contains(b, "\x03") {
			seen = true
			continue
		}
		if seen && parts[1] == "read" {
			after.WriteString(b)
		}
	}
	a := strings.ReplaceAll(after.String(), endMark, "")
	return len(a), eraseRE.MatchString(a)
}

func wilson(k, n int) (float64, float64) {
	if n == 0 {
		return 0, 0
	}
	z, p, nn := 1.96, float64(k)/float64(n), float64(n)
	d := 1 + z*z/nn
	c := (p + z*z/(2*nn)) / d
	h := z * math.Sqrt(p*(1-p)/nn+z*z/(4*nn*nn)) / d
	return math.Max(0, c-h), math.Min(1, c+h)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
