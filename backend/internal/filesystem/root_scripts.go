package filesystem

const rootBeginMarker = "ROAMINAL_FILESYSTEM_ROOT_V1"

const tmuxRootScript = `set -eu
session_name=$1
if ! command -v tmux >/dev/null 2>&1; then exit 20; fi
tmux has-session -t "=$session_name:"
path=$(tmux display-message -p -t "=$session_name:" '#{pane_current_path}')
case "$path" in /*) ;; *) exit 21;; esac
path=$(cd -- "$path" && pwd -P)
test -d "$path"
printf '%s\0%s\0' 'ROAMINAL_FILESYSTEM_ROOT_V1' "$path"
`

const configuredRootScript = `set -eu
value=$1
case "$value" in
  '$HOME') candidate=$HOME ;;
  '~') candidate=$HOME ;;
  '$HOME/'*) candidate=$HOME/${value#'$HOME/'} ;;
  '~/'*) candidate=$HOME/${value#'~/'} ;;
  /*) candidate=$value ;;
  *) exit 22 ;;
esac
path=$(cd -- "$candidate" && pwd -P)
test -d "$path"
printf '%s\0%s\0' 'ROAMINAL_FILESYSTEM_ROOT_V1' "$path"
`
