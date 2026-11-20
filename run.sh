#!/usr/bin/env bash
# Runs the matrix and writes results/<os>/: system.txt, one summary .txt per load, and out-<load>/ (runs.csv
# and every pty log). N runs per binary (default 100). Each load line takes every core for several minutes.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$PATH:/usr/sbin:/sbin"
N="${N:-100}"
EXE=$(go env GOEXE)
OS=$(go env GOOS)
R=results/$OS; mkdir -p $R
load() { case $OS in darwin) sysctl -n vm.loadavg ;; linux) cut -d' ' -f1-3 /proc/loadavg ;; *) echo n/a ;; esac; }
{
  echo "date:   $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "go:     $(go version)"
  case $OS in
    darwin)  echo "os:     macOS $(sw_vers -productVersion) ($(sw_vers -buildVersion)), $(uname -srm)"
             echo "cpu:    $(sysctl -n machdep.cpu.brand_string), $(sysctl -n hw.ncpu) logical, $(( $(sysctl -n hw.memsize) / 1073741824 )) GB" ;;
    linux)   echo "os:     $(. /etc/os-release && echo "$PRETTY_NAME"), $(uname -srm)"
             echo "cpu:    $(lscpu | sed -n 's/^Model name: *//p'), $(nproc) logical, $(free -g | awk '/Mem:/ {print $2}') GB" ;;
    windows) echo "os:     $(cmd //c ver | tr -d '\r' | sed '/^$/d')"
             echo "cpu:    $(powershell -NoProfile -Command '(Get-CimInstance Win32_Processor).Name' | tr -d '\r'), $NUMBER_OF_PROCESSORS logical"
             echo "pwsh:   $(powershell -NoProfile -Command '$PSVersionTable.PSVersion.ToString()' | tr -d '\r')" ;;
  esac
  echo "pty:    40x24, $( [ $OS = windows ] && echo ConPTY || echo Unix pty ) via charmbracelet/x/xpty v0.1.4, screen via charmbracelet/x/vt v0.1.0"
  echo "binaries:"; for b in bin/*; do printf '  %s  ' "$(basename "$b")"; go version -m "$b" | awk '/dep/ && /bubbletea|huh\/v2|bubbles\/v2/ {printf "%s@%s ", $2, $3}'; echo; done
} > $R/system.txt
cat $R/system.txt
ALL=$(cd bin && ls | grep -v '^drive' | sed "s/$EXE\$//" | sort -V)
SOME=$(echo "$ALL" | grep -E 'v2\.0\.2$|v2\.1\.1$|v2\.1\.2$')
for L in ${LOADS:-inproc ext none}; do
  set -- ; for b in $( [ $L = inproc ] && echo "$ALL" || echo "$SOME" ); do set -- "$@" "./$b$EXE"; done
  echo "== load=$L  loadavg before: $(load)" | tee -a $R/system.txt
  (cd bin && ./drive$EXE -n $N -load $L -out ../$R/out-$L "$@") | tee $R/$L.txt
  echo "== load=$L  loadavg after:  $(load)" | tee -a $R/system.txt
done
