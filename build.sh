#!/usr/bin/env bash
# Builds every repro binary into bin/: for each bubbletea version, the huh form (huh v2.0.3, bubbles v2.2.1
# where the version allows it) and the minimal program in two modes (tea.Interrupt and tea.Quit), plus the driver.
set -euo pipefail
cd "$(dirname "$0")"
VERSIONS="${VERSIONS:-v2.0.2 v2.0.10 v2.1.0 v2.1.1 v2.1.2}"
EXE=$(go env GOEXE)
mkdir -p bin build
(cd driver && go build -o ../bin/drive$EXE .)
for v in $VERSIONS; do
  d=build/huh-$v; mkdir -p $d; cp huhform/main.go $d/
  printf 'module huh%s\n\ngo 1.24\n' "${v//./}" > $d/go.mod
  (cd $d && go get charm.land/huh/v2@v2.0.3 charm.land/bubbles/v2@v2.2.1 charm.land/bubbletea/v2@$v >/dev/null 2>&1 \
     || go get charm.land/huh/v2@v2.0.3 charm.land/bubbletea/v2@$v >/dev/null 2>&1
   go mod tidy >/dev/null 2>&1 && go build -o ../../bin/huh-$v$EXE .)
  d=build/mini-$v; mkdir -p $d; cp mini/main.go $d/
  printf 'module mini%s\n\ngo 1.24\n' "${v//./}" > $d/go.mod
  (cd $d && go get charm.land/bubbletea/v2@$v >/dev/null 2>&1 && go mod tidy >/dev/null 2>&1 \
   && go build -ldflags "-X main.mode=interrupt" -o ../../bin/mini-int-$v$EXE . \
   && go build -ldflags "-X main.mode=quit" -o ../../bin/mini-quit-$v$EXE .)
done
echo "built:"; for b in bin/*; do printf '  %s  ' "$b"; go version -m "$b" | awk '/dep/ && /bubbletea|huh\/v2|bubbles\/v2/ {printf "%s@%s ", $2, $3}'; echo; done
