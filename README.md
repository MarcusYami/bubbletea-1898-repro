# bubbletea#1898 reproduction

A reproduction harness and full results for
[charmbracelet/bubbletea#1898](https://github.com/charmbracelet/bubbletea/issues/1898): after ctrl-c, a huh
form's menu is sometimes left on screen and sometimes erased.

Public modules only (bubbletea, huh, bubbles, charmbracelet/x/xpty, charmbracelet/x/vt). Go 1.24 or later.
Runs on macOS, Linux and Windows (Git bash, ConPTY).

## Findings

1. **Not new in v2.1.x.** Every version tested, v2.0.2 to v2.1.2, behaves the same; v2.1.2 does not change it.
2. **`tea.Quit` is deterministic.** A program whose View is empty after ctrl-c and that returns `tea.Quit`
   erased its menu in 100 of 100 runs, on every version, OS and load.
3. **`tea.Interrupt` never draws a final frame.** The same program returning `tea.Interrupt` (which is what huh
   v2.0.3's default `CancelCmd` is) leaves whatever the renderer last flushed. With no added load it kept the
   frame in every run. Under load, the renderer's ticker sometimes flushes the now-empty view before shutdown, and then the menu
   is erased.
4. **huh shows it** as an erased form in 3 to 15% of runs, depending on OS and load, including on Linux with no
   added load.
5. **The pty logs show the mechanism.** In 21 of the 24 erased huh runs on macOS (v2.1.1 and v2.1.2, in-process
   load), the program's first output after ctrl-c is a cursor-up plus clear (`ESC[5A ESC[J`), the ticker
   flushing the empty view. That is never the first output in a run that kept the menu.
6. **"An erase sequence was written" does not tell the two cases apart.** Every run writes `ESC[J` after the key,
   including every run that kept the menu, because shutdown clears the cursor line.

### Why (from reading the v2.1.1 source)

On `InterruptMsg` the event loop returns `ErrInterrupted`. With a non-nil error the program counts as killed, so
it skips the final `render(model)` and calls `shutdown(killed=true)`, whose renderer stop skips the final
`flush`. The screen then holds the last frame the ticker flushed: the menu if the ticker had not run since the
model went empty, nothing if it had. `tea.Quit` takes the graceful path, renders the final (empty) view and
flushes it, so it always erases.

## Results

100 runs per binary and load. Cells are **menu erased** / **menu kept** counts.

For `mini-int`, "kept" means the menu stayed with its last line cleared (`partial-3of4` in the CSV): shutdown's
`CR ESC[J` clears the cursor line and the error text is printed there. huh's frame ends in a blank line, so the
same clear leaves all of its menu.

| Program | Version | macOS in-process | macOS external | macOS none | Linux in-process | Linux external | Linux none |
|---|---|---|---|---|---|---|---|
| huh | v2.0.2 | 12 / 88 | 0 / 100 | 0 / 100 | 8 / 92 | 12 / 88 | 4 / 96 |
| huh | v2.0.10 | 12 / 88 | | | 5 / 95 | | |
| huh | v2.1.0 | 9 / 91 | | | 5 / 95 | | |
| huh | v2.1.1 | 12 / 88 | 0 / 100 | 0 / 100 | 5 / 95 | 15 / 85 | 3 / 97 |
| huh | v2.1.2 | 12 / 88 | 0 / 100 | 0 / 100 | 6 / 94 | 8 / 92 | 6 / 94 |
| mini, `tea.Interrupt` | v2.0.2 | 14 / 86 | 0 / 100 | 0 / 100 | 4 / 96 | 8 / 92 | 0 / 100 |
| mini, `tea.Interrupt` | v2.0.10 | 17 / 83 | | | 7 / 93 | | |
| mini, `tea.Interrupt` | v2.1.0 | 12 / 88 | | | 7 / 93 | | |
| mini, `tea.Interrupt` | v2.1.1 | 8 / 92 | 0 / 100 | 0 / 100 | 12 / 88 | 17 / 83 | 0 / 100 |
| mini, `tea.Interrupt` | v2.1.2 | 8 / 92 | 0 / 100 | 0 / 100 | 7 / 93 | 11 / 89 | 0 / 100 |
| mini, `tea.Quit` | all five | 100 / 0 | 100 / 0 | 100 / 0 | 100 / 0 | 100 / 0 | 100 / 0 |

Windows results will be added here when that run finishes.

The external and no-load lines ran v2.0.2, v2.1.1 and v2.1.2 only. Wilson 95% intervals for every cell are in
`results/<os>/<load>.txt`; at 100 runs, 12 erased is 7.0% to 19.8%, and 0 erased is 0% to 3.7%.

### Systems

| | macOS | Linux |
|---|---|---|
| OS | macOS 27.2 (26B123), arm64 | Debian 13, kernel 6.12, x86_64 |
| CPU | Apple M5 Pro, 15 logical (5 performance + 10 efficiency) | Intel Core i5-14500, 20 logical |
| Memory | 64 GB | 31 GB |
| Go | 1.27.3 | 1.27.2 |
| Terminal | 40x24 Unix pty, x/vt screen | 40x24 Unix pty, x/vt screen |
| Other load | Shared machine: 1-min load average about 25 to 45 from other work during every line, "none" included | A media transcoder (about 1.3 cores). 1-min load average 52 before the first line (other work on the host finishing); the readings around each line are in `system.txt` |

Full details per run, including every module version in each binary, are in `results/<os>/system.txt`.

## What the harness does

Each run starts a program at a 40x24 pty whose output feeds a terminal emulator (x/vt, which also answers the
program's terminal queries as a real terminal does). It waits until the menu is drawn, sends ctrl-c, waits for
the exit, and records:

- **result**: what the screen shows at the end. `kept` means all four menu lines, `erased` none, and
  `partial-KofN` some.
- **final_screen**: the screen text itself.
- **bytes_after_key** and **erase_after_key**: what the program wrote after ctrl-c, and whether it held an
  erase (CSI J or K).
- **view_before_size** (mini only): whether View ran before the first WindowSizeMsg. It did in every run.

Runs interleave the binaries (run 1 of each, then run 2 of each, ...) so load drift hits all alike.

### Programs

- `huh-<ver>`: the issue's repro, a huh v2.0.3 `Form.Run` with one Select (bubbles v2.2.1; v2.0.2 keeps bubbles
  v2.0.0, the newest that version allows).
- `mini-int-<ver>`: bubbletea only. A four-line menu. On ctrl-c the model marks itself done, so its View becomes
  empty as a finished huh form's does, and returns `tea.Interrupt`.
- `mini-quit-<ver>`: the same, returning `tea.Quit`.

### Load

- `none`: no added load.
- `inproc`: 3 busy goroutines per CPU inside the driver process.
- `ext`: 10 separate busy processes, each with a busy goroutine per CPU.

## Run it

    ./build.sh           # builds bin/ (VERSIONS="v2.1.1 v2.1.2" ./build.sh for fewer)
    N=100 ./run.sh       # writes results/<os>/; LOADS="inproc" picks load lines

The output is:

- `results/<os>/system.txt`: the system details and the load average around each line.
- `results/<os>/<load>.txt`: the summary per binary.
- `results/<os>/out-<load>/runs.csv`: one row per run.
- `results/<os>/out-<load>/logs/`: every run's pty log, each read and write with its UTC time.

Each load line takes every core for several minutes.

The published logs are in `results/<os>/logs.tar.gz`. Their times were recorded in local time and converted to
UTC for publishing; the driver now writes UTC itself.
