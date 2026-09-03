package filesystem

import (
	"bytes"
	"mime"
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
	MIMEType   string
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
	result := make([]rawEntry, 0)
	for index := 0; index < len(records); {
		// Directory frames contain seven fields, while stat frames append the
		// detected MIME field before the empty record terminator.
		width := 0
		mimeType := ""
		switch {
		case index+7 <= len(records) && len(records[index+6]) == 0:
			width = 7
		case index+8 <= len(records) && len(records[index+7]) == 0:
			width = 8
			parsed, mimeErr := parseOptionalMIME(records[index+6])
			if mimeErr != nil {
				return nil, ErrProtocol
			}
			mimeType = parsed
		default:
			return nil, ErrProtocol
		}
		if len(result) >= maxDirectoryEntries {
			return nil, ErrDirectoryTooLarge
		}
		record := records[index : index+width]
		name := string(record[0])
		if name == "" || !utf8.Valid(record[0]) || bytes.Contains(record[0], []byte{'/'}) {
			return nil, ErrFilenameEncoding
		}
		entryType := string(record[1])
		if entryType != "directory" && entryType != "file" && entryType != "symlink" && entryType != "other" {
			return nil, ErrProtocol
		}
		size, err := parseOptionalInt64(record[2])
		if err != nil || (size != nil && *size < 0) {
			return nil, ErrProtocol
		}
		modifiedUnix, err := parseOptionalInt64(record[3])
		if err != nil {
			return nil, ErrProtocol
		}
		var modifiedAt *time.Time
		if modifiedUnix != nil {
			value := time.Unix(*modifiedUnix, 0).UTC()
			modifiedAt = &value
		}
		mode, err := strconv.ParseUint(string(record[4]), 10, 32)
		if err != nil {
			return nil, ErrProtocol
		}
		symlink, err := strconv.ParseBool(string(record[5]))
		if err != nil || (entryType == "symlink") != symlink {
			return nil, ErrProtocol
		}
		if len(record) == 7 && string(record[6]) != "" {
			return nil, ErrProtocol
		}
		if len(record) == 8 && string(record[7]) != "" {
			return nil, ErrProtocol
		}
		result = append(result, rawEntry{Name: name, Type: entryType, MIMEType: mimeType, Size: size, ModifiedAt: modifiedAt, Mode: uint32(mode), Symlink: symlink})
		index += width
	}
	return result, nil
}

func parseOptionalMIME(value []byte) (string, error) {
	if bytes.Equal(value, []byte("-")) {
		return "", nil
	}
	if len(value) == 0 || !utf8.Valid(value) || bytes.IndexAny(value, "\x00\r\n") >= 0 {
		return "", ErrProtocol
	}
	parsed, _, err := mime.ParseMediaType(string(value))
	if err != nil {
		return "", ErrProtocol
	}
	return parsed, nil
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
mime_type=-
if [ "$type" = file ] && command -v file >/dev/null 2>&1; then
  mime_type=$(file -b --mime-type -- "$target" 2>/dev/null || true)
  case "$mime_type" in
    ''|*[!A-Za-z0-9./+_-]*) mime_type=-;;
  esac
fi
metadata=$(stat -c '%s %Y %a' -- "$target" 2>/dev/null || stat -f '%z %m %Lp' "$target" 2>/dev/null || printf '%s' '- - -')
set -- $metadata
size=${1:--}; modified=${2:--}; mode=${3:--}
case "$size" in ''|*[!0-9]*) size=-;; esac
case "$modified" in ''|*[!0-9-]*) modified=-;; esac
case "$mode" in ''|*[!0-9]*) mode=0;; esac
printf '%s\0' 'ROAMINAL_FILESYSTEM_V1_BEGIN'
printf '%s\0%s\0%s\0%s\0%s\0%s\0%s\0%s\0' "$name" "$type" "$size" "$modified" "$mode" "$symlink" "$mime_type" ''
printf '%s\0' 'ROAMINAL_FILESYSTEM_V1_END'
`

const contentScript = `set -eu
root=$1
relative=$2
start=$3
length=$4
root_real=$(cd -- "$root" && pwd -P)
target=$root_real
if [ "$relative" != "." ]; then target=$root_real/$relative; fi
if [ -L "$target" ] || [ ! -f "$target" ]; then exit 24; fi
parent=$(dirname -- "$target")
parent_real=$(cd -- "$parent" && pwd -P)
case "$parent_real" in
  "$root_real"|"$root_real"/*) ;;
  *) exit 23 ;;
esac
target=$parent_real/${target##*/}
if [ "$length" -gt 0 ]; then
  dd if="$target" bs=1 skip="$start" count="$length" 2>/dev/null
fi
`
