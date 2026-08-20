package filesystem

const (
	rsyncAvailableMarker = "ROAMINAL_RSYNC_AVAILABLE"
	uploadTargetMarker   = "ROAMINAL_FILESYSTEM_UPLOAD_TARGET_V1"
	uploadConflictMarker = "ROAMINAL_FILESYSTEM_UPLOAD_CONFLICTS_V1"
	uploadConflictEnd    = "ROAMINAL_FILESYSTEM_UPLOAD_CONFLICTS_END"
)

const rsyncProbeScript = `if command -v rsync >/dev/null 2>&1; then printf '%s' 'ROAMINAL_RSYNC_AVAILABLE'; else printf '%s' 'ROAMINAL_RSYNC_UNAVAILABLE'; fi`

const uploadTargetScript = `set -eu
root=$1
relative=$2
root_real=$(cd -- "$root" && pwd -P)
target=$root_real
if [ "$relative" != "." ]; then target=$root_real/$relative; fi
target_real=$(cd -- "$target" && pwd -P)
case "$target_real" in
  "$root_real"|"$root_real"/*) ;;
  *) exit 23 ;;
esac
test -d "$target_real"
printf '%s\0%s\0' 'ROAMINAL_FILESYSTEM_UPLOAD_TARGET_V1' "$target_real"
`

const uploadConflictScript = `set -eu
root=$1
relative=$2
shift 2
root_real=$(cd -- "$root" && pwd -P)
target=$root_real
if [ "$relative" != "." ]; then target=$root_real/$relative; fi
target_real=$(cd -- "$target" && pwd -P)
case "$target_real" in
  "$root_real"|"$root_real"/*) ;;
  *) exit 23 ;;
esac
printf '%s\0' 'ROAMINAL_FILESYSTEM_UPLOAD_CONFLICTS_V1'
for file in "$@"; do
  candidate=$target_real/$file
  if [ -e "$candidate" ] || [ -L "$candidate" ]; then printf '%s\0' "$file"; fi
done
printf '%s\0' 'ROAMINAL_FILESYSTEM_UPLOAD_CONFLICTS_END'
`

const remoteMkdirScript = `set -eu
root=$1
relative=$2
shift 2
root_real=$(cd -- "$root" && pwd -P)
target=$root_real
if [ "$relative" != "." ]; then target=$root_real/$relative; fi
target_real=$(cd -- "$target" && pwd -P)
case "$target_real" in
  "$root_real"|"$root_real"/*) ;;
  *) exit 23 ;;
esac
for directory in "$@"; do
  mkdir -p -- "$target_real/$directory"
done
`

const remoteMtimeScript = `set -eu
target=$1
relative=$2
local_mtime=$3
candidate=$target/$relative
if [ ! -e "$candidate" ] && [ ! -L "$candidate" ]; then printf '%s' upload; exit 0; fi
if [ "$local_mtime" -le 0 ]; then printf '%s' upload; exit 0; fi
remote_mtime=$(stat -c '%Y' -- "$candidate" 2>/dev/null || stat -f '%m' "$candidate" 2>/dev/null || printf '%s' 0)
case "$remote_mtime" in
  ''|*[!0-9-]*) printf '%s' upload ;;
  *) if [ "$remote_mtime" -ge "$local_mtime" ]; then printf '%s' skip; else printf '%s' upload; fi ;;
esac
`
