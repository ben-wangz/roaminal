package filesystem

import (
	"bytes"
	"strconv"
	"time"
	"unicode/utf8"
)

const (
	directoryBegin      = "ROAMINAL_FILESYSTEM_V1_BEGIN"
	directoryEnd        = "ROAMINAL_FILESYSTEM_V1_END"
	maxDirectoryEntries = 10_000
	maxDirectoryOutput  = 16 << 20
)

type rawEntry struct {
	Name       string
	Type       string
	Size       *int64
	ModifiedAt *time.Time
	Mode       uint32
	Symlink    bool
}

func parseDirectory(data []byte) ([]rawEntry, error) {
	fields := bytes.Split(data, []byte{0})
	if len(fields) < 3 || string(fields[0]) != directoryBegin || string(fields[len(fields)-2]) != directoryEnd || len(fields[len(fields)-1]) != 0 {
		return nil, ErrProtocol
	}
	records := fields[1 : len(fields)-2]
	if len(records)%7 != 0 {
		return nil, ErrProtocol
	}
	if len(records)/7 > maxDirectoryEntries {
		return nil, ErrDirectoryTooLarge
	}
	result := make([]rawEntry, 0, len(records)/7)
	for index := 0; index < len(records); index += 7 {
		name := string(records[index])
		if name == "" || !utf8.Valid(records[index]) || bytes.Contains(records[index], []byte{'/'}) {
			return nil, ErrFilenameEncoding
		}
		entryType := string(records[index+1])
		if entryType != "directory" && entryType != "file" && entryType != "symlink" && entryType != "other" {
			return nil, ErrProtocol
		}
		size, err := parseOptionalInt64(records[index+2])
		if err != nil || (size != nil && *size < 0) {
			return nil, ErrProtocol
		}
		modifiedUnix, err := parseOptionalInt64(records[index+3])
		if err != nil {
			return nil, ErrProtocol
		}
		var modifiedAt *time.Time
		if modifiedUnix != nil {
			value := time.Unix(*modifiedUnix, 0).UTC()
			modifiedAt = &value
		}
		mode, err := strconv.ParseUint(string(records[index+4]), 10, 32)
		if err != nil {
			return nil, ErrProtocol
		}
		symlink, err := strconv.ParseBool(string(records[index+5]))
		if err != nil || (entryType == "symlink") != symlink {
			return nil, ErrProtocol
		}
		if string(records[index+6]) != "" {
			return nil, ErrProtocol
		}
		result = append(result, rawEntry{Name: name, Type: entryType, Size: size, ModifiedAt: modifiedAt, Mode: uint32(mode), Symlink: symlink})
	}
	return result, nil
}

func parseOptionalInt64(value []byte) (*int64, error) {
	if bytes.Equal(value, []byte("-")) {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

const directoryScript = `set -eu
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
printf '%s\0' 'ROAMINAL_FILESYSTEM_V1_BEGIN'
for entry in "$target_real"/* "$target_real"/.[!.]* "$target_real"/..?*; do
  [ -e "$entry" ] || [ -L "$entry" ] || continue
  name=${entry##*/}
  type=other
  symlink=false
  if [ -L "$entry" ]; then type=symlink; symlink=true
  elif [ -d "$entry" ]; then type=directory
  elif [ -f "$entry" ]; then type=file
  fi
  metadata=$(stat -c '%s %Y %a' -- "$entry" 2>/dev/null || stat -f '%z %m %Lp' "$entry" 2>/dev/null || printf '%s' '- - -')
  set -- $metadata
  size=${1:--}; modified=${2:--}; mode=${3:--}
  case "$size" in ''|*[!0-9]*) size=-;; esac
  case "$modified" in ''|*[!0-9-]*) modified=-;; esac
  case "$mode" in ''|*[!0-9]*) mode=0;; esac
  printf '%s\0%s\0%s\0%s\0%s\0%s\0%s\0' "$name" "$type" "$size" "$modified" "$mode" "$symlink" ''
done
printf '%s\0' 'ROAMINAL_FILESYSTEM_V1_END'
`

const statScript = `set -eu
root=$1
relative=$2
root_real=$(cd -- "$root" && pwd -P)
target=$root_real
if [ "$relative" != "." ]; then target=$root_real/$relative; fi
if [ ! -e "$target" ] && [ ! -L "$target" ]; then exit 24; fi
parent=$(dirname -- "$target")
parent_real=$(cd -- "$parent" && pwd -P)
case "$parent_real" in
  "$root_real"|"$root_real"/*) ;;
  *) exit 23 ;;
esac
target=$parent_real/${target##*/}
name=${target##*/}
if [ "$relative" = "." ]; then name=.; fi
type=other
symlink=false
if [ -L "$target" ]; then type=symlink; symlink=true
elif [ -d "$target" ]; then type=directory
elif [ -f "$target" ]; then type=file
fi
metadata=$(stat -c '%s %Y %a' -- "$target" 2>/dev/null || stat -f '%z %m %Lp' "$target" 2>/dev/null || printf '%s' '- - -')
set -- $metadata
size=${1:--}; modified=${2:--}; mode=${3:--}
case "$size" in ''|*[!0-9]*) size=-;; esac
case "$modified" in ''|*[!0-9-]*) modified=-;; esac
case "$mode" in ''|*[!0-9]*) mode=0;; esac
printf '%s\0%s\0%s\0%s\0%s\0%s\0%s\0' "$name" "$type" "$size" "$modified" "$mode" "$symlink" ''
printf '%s\0' 'ROAMINAL_FILESYSTEM_V1_END'
`
