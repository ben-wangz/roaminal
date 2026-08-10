package connection

const remoteCollectorScript = `#!/bin/sh
set -u
nonce=$1
printf 'ROAMINAL_MONITOR_V1_BEGIN_%s\n' "$nonce"
scope=unknown
cg=/sys/fs/cgroup
has_cgroup=0
grep -qE '^[^:]*:[^:]*:' /proc/self/cgroup 2>/dev/null && has_cgroup=1
v2info=$(awk -F' - ' '$2 ~ /^cgroup2[[:space:]]/ {print $1; exit}' /proc/self/mountinfo 2>/dev/null || true)
v2root=$(printf '%s\n' "$v2info" | awk '{print $4}')
v2mount=$(printf '%s\n' "$v2info" | awk '{print $5}')
[ -n "$v2mount" ] && cg=$v2mount
if [ -r "$cg/cpu.stat" ] || [ -r "$cg/memory.current" ]; then
  path=$(awk -F: '$1 == "0" {print $3; exit}' /proc/self/cgroup 2>/dev/null || true)
  if [ -n "$v2root" ] && [ "$v2root" != "/" ]; then
    case "$path" in ("$v2root"|"$v2root"/*) path=${path#"$v2root"};; (*) path=;; esac
  fi
  case "$path" in (/*..*) path=;; esac
  [ -n "$path" ] && [ "$path" != "/" ] && cg="$cg$path"
  usage=$(awk '$1 == "usage_usec" {print $2; exit}' "$cg/cpu.stat" 2>/dev/null || true)
  current=$(cat "$cg/memory.current" 2>/dev/null || true)
  if [ -n "$usage" ] || [ -n "$current" ]; then
    scope=cgroup-v2
    if [ -n "$usage" ]; then
      printf 'cpu_usage_ns=%s\n' "$((usage * 1000))"
      max=$(awk '{print $1}' "$cg/cpu.max" 2>/dev/null || true)
      period=$(awk '{print $2}' "$cg/cpu.max" 2>/dev/null || true)
      cpuset=$(cat "$cg/cpuset.cpus.effective" 2>/dev/null || true)
      set_capacity=$(printf '%s\n' "$cpuset" | awk -F, 'NF {for (i=1; i<=NF; i++) {split($i, r, "-"); if (r[2] != "") n += r[2] - r[1] + 1; else n++}} END {if (n > 0) print n * 1000}' 2>/dev/null || true)
      case "$max:$period" in
        (max:*|'':*) [ -n "$set_capacity" ] && printf 'cpu_capacity_milli=%s\n' "$set_capacity";;
        (*:0|'':0) [ -n "$set_capacity" ] && printf 'cpu_capacity_milli=%s\n' "$set_capacity";;
        (*) quota_capacity=$((max * 1000 / period)); if [ -n "$set_capacity" ] && [ "$set_capacity" -lt "$quota_capacity" ]; then quota_capacity=$set_capacity; fi; printf 'cpu_capacity_milli=%s\n' "$quota_capacity";;
      esac
    fi
    if [ -n "$current" ]; then
      printf 'memory_current_bytes=%s\n' "$current"
      inactive=$(awk '$1 == "inactive_file" {print $2; exit}' "$cg/memory.stat" 2>/dev/null || true)
      [ -n "$inactive" ] && printf 'memory_inactive_file_bytes=%s\n' "$inactive"
      limit=$(cat "$cg/memory.max" 2>/dev/null || true)
      case "$limit" in (''|max) ;; (*) printf 'memory_limit_bytes=%s\n' "$limit";; esac
    fi
  fi
fi
if [ "$scope" = unknown ] && [ "$has_cgroup" = 1 ]; then
  cpu_info=$(awk -F' - ' '$2 ~ /^cgroup[[:space:]]/ && $2 ~ /(^|,)cpuacct(,|[[:space:]])/ {print $1; exit}' /proc/self/mountinfo 2>/dev/null || true)
  cpu_root=$(printf '%s\n' "$cpu_info" | awk '{print $4}')
  cpu_mount=$(printf '%s\n' "$cpu_info" | awk '{print $5}')
  [ -z "$cpu_mount" ] && cpu_mount=/sys/fs/cgroup/cpu,cpuacct
  memory_info=$(awk -F' - ' '$2 ~ /^cgroup[[:space:]]/ && $2 ~ /(^|,)memory(,|[[:space:]])/ {print $1; exit}' /proc/self/mountinfo 2>/dev/null || true)
  memory_root=$(printf '%s\n' "$memory_info" | awk '{print $4}')
  memory_mount=$(printf '%s\n' "$memory_info" | awk '{print $5}')
  [ -z "$memory_mount" ] && memory_mount=/sys/fs/cgroup/memory
  cpu_path=$(awk -F: '$2 ~ /(^|,)cpuacct(,|$)/ {print $3; exit}' /proc/self/cgroup 2>/dev/null || true)
  memory_path=$(awk -F: '$2 ~ /(^|,)memory(,|$)/ {print $3; exit}' /proc/self/cgroup 2>/dev/null || true)
  case "$cpu_path:$memory_path" in (*..*) cpu_path=; memory_path=;; esac
  if [ -n "$cpu_root" ] && [ "$cpu_root" != "/" ]; then case "$cpu_path" in ("$cpu_root"|"$cpu_root"/*) cpu_path=${cpu_path#"$cpu_root"};; (*) cpu_path=;; esac; fi
  if [ -n "$memory_root" ] && [ "$memory_root" != "/" ]; then case "$memory_path" in ("$memory_root"|"$memory_root"/*) memory_path=${memory_path#"$memory_root"};; (*) memory_path=;; esac; fi
  [ -n "$cpu_path" ] && [ "$cpu_path" != "/" ] && cpu_mount="$cpu_mount$cpu_path"
  [ -n "$memory_path" ] && [ "$memory_path" != "/" ] && memory_mount="$memory_mount$memory_path"
  usage=$(cat "$cpu_mount/cpuacct.usage" 2>/dev/null || true)
  current=$(cat "$memory_mount/memory.usage_in_bytes" 2>/dev/null || true)
  if [ -n "$usage" ] && [ -n "$current" ]; then
    scope=cgroup-v1
    printf 'cpu_usage_ns=%s\n' "$usage"
    quota=$(cat "$cpu_mount/cpu.cfs_quota_us" 2>/dev/null || true)
    period=$(cat "$cpu_mount/cpu.cfs_period_us" 2>/dev/null || true)
    case "$quota:$period" in (-1:*|'':*) ;; (*:0|'':0) ;; (*) printf 'cpu_capacity_milli=%s\n' "$((quota * 1000 / period))";; esac
    printf 'memory_current_bytes=%s\n' "$current"
    inactive=$(awk '$1 == "total_inactive_file" || $1 == "inactive_file" {print $2; exit}' "$memory_mount/memory.stat" 2>/dev/null || true)
    [ -n "$inactive" ] && printf 'memory_inactive_file_bytes=%s\n' "$inactive"
    limit=$(cat "$memory_mount/memory.limit_in_bytes" 2>/dev/null || true)
    case "$limit" in (''|9223372036854771712|9223372036854775807) ;; (*) printf 'memory_limit_bytes=%s\n' "$limit";; esac
  fi
fi
if [ "$scope" = unknown ] && [ "$has_cgroup" = 0 ] && [ -r /proc/stat ]; then
  line=$(awk '$1 == "cpu" {print; exit}' /proc/stat 2>/dev/null || true)
  set -- $line
  if [ "$#" -ge 5 ]; then
    total=0; i=2
    while [ "$i" -le "$#" ]; do eval "value=\${$i}"; total=$((total + value)); i=$((i + 1)); done
    idle=$(( $5 + ${6:-0} ))
    scope=host
    printf 'host_cpu_total_ticks=%s\n' "$total"
    printf 'host_cpu_idle_ticks=%s\n' "$idle"
  fi
fi
if [ "$scope" = host ] && [ -r /proc/meminfo ]; then
  total=$(awk '$1 == "MemTotal:" {print $2 * 1024; exit}' /proc/meminfo)
  available=$(awk '$1 == "MemAvailable:" {print $2 * 1024; exit}' /proc/meminfo)
  [ -n "$total" ] && [ -n "$available" ] && printf 'host_memory_total_bytes=%s\nhost_memory_available_bytes=%s\n' "$total" "$available"
fi
uptime=$(awk '{print $1; exit}' /proc/uptime 2>/dev/null || true)
stat=$(cat /proc/1/stat 2>/dev/null || true)
rest=${stat##*) }
set -- $rest
start_ticks=${20:-}
clock_ticks=$(getconf CLK_TCK 2>/dev/null || true)
[ -n "$start_ticks" ] && printf 'pid1_start_ticks=%s\n' "$start_ticks"
[ -n "$clock_ticks" ] && printf 'clock_ticks_per_second=%s\n' "$clock_ticks"
[ -n "$uptime" ] && printf 'system_uptime_seconds=%s\n' "$uptime"
load=$(cat /proc/loadavg 2>/dev/null || true)
set -- $load
[ "$#" -ge 3 ] && printf 'load_1=%s\nload_5=%s\nload_15=%s\n' "$1" "$2" "$3"
disk=$(LC_ALL=C df -k -P / 2>/dev/null | awk 'NR > 1 {print $(NF-4), $(NF-3), $(NF-2), $(NF-1); exit}')
set -- $disk
if [ "$#" -ge 4 ]; then
  percent=${4%%%}
  case "$percent" in (*[!0-9.]*) ;; (*) printf 'rootfs_total_kib=%s\nrootfs_used_kib=%s\nrootfs_available_kib=%s\nrootfs_capacity_percent=%s\n' "$1" "$2" "$3" "$percent";; esac
fi
printf 'scope=%s\n' "$scope"
printf 'ROAMINAL_MONITOR_V1_END_%s\n' "$nonce"
`
