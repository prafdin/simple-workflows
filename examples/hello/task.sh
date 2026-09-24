#!/bin/sh
field() {
  printf '%-11s%s\n' "$1:" "$2"
}
readable() {
  if [ -r "$1" ]; then cat "$1"; else echo unknown; fi
}
cpu() {
  [ -r /sys/fs/cgroup/cpu.max ] || { echo unknown; return; }
  read -r quota period < /sys/fs/cgroup/cpu.max
  if [ "$quota" = max ]; then echo unlimited; else awk -v q="$quota" -v p="$period" 'BEGIN { printf "%.2f cores\n", q / p }'; fi
}
memory() {
  [ -r /sys/fs/cgroup/memory.max ] || { echo unknown; return; }
  limit=$(cat /sys/fs/cgroup/memory.max)
  if [ "$limit" = max ]; then echo unlimited; else echo "$((limit / 1048576)) MiB"; fi
}
field pod "$(hostname)"
field ip "$(hostname -i 2>/dev/null || echo unknown)"
field namespace "$(readable /var/run/secrets/kubernetes.io/serviceaccount/namespace)"
field "cpu limit" "$(cpu)"
field "mem limit" "$(memory)"
field kernel "$(uname -r)"
field time "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sleep 10
echo done
