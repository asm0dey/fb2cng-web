#!/bin/sh
# Fake fbc for tests. Emulates: `dumpconfig --default` and
# `[-c cfg] convert --to FMT [--overwrite --nd] INPUT DEST`.
mode=""
for a in "$@"; do
  case "$a" in
    dumpconfig) mode=dump ;;
    convert) mode=convert ;;
  esac
done

if [ "$mode" = "dump" ]; then
  # Find the destination file: last arg when more than just "dumpconfig --default"
  dest_file=""
  prev=""
  for a in "$@"; do
    case "$a" in
      dumpconfig|--default) ;;
      *) dest_file="$a" ;;
    esac
    prev="$a"
  done
  yaml='version: 1\ndocument:\n    toc_type: normal\n'
  if [ -n "$dest_file" ]; then
    printf "$yaml" > "$dest_file"
  else
    printf "$yaml"
  fi
  exit 0
fi

if [ "$mode" = "convert" ]; then
  to=""
  positionals=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --to) to="$2"; shift 2; continue ;;
      -c) shift 2; continue ;;
      convert|--overwrite|--ow|--nd|--nodirs|-d|--debug) shift; continue ;;
      *) positionals="$positionals $1"; shift ;;
    esac
  done
  # positionals = INPUT ... DEST  (last is dest, first is input)
  # NOTE: positional parsing uses word-splitting; test paths must not contain spaces (t.TempDir paths don't).
  # shellcheck disable=SC2086
  set -- $positionals
  input=$1
  eval dest=\${$#}
  base=$(basename "$input"); base=${base%.*}
  case "$to" in
    epub2|epub3|"") ext="epub" ;;
    kepub) ext="kepub.epub" ;;
    *) ext="$to" ;;
  esac
  if echo "$input" | grep -q corrupt; then
    echo "fake-fbc: cannot parse $input" >&2
    exit 1
  fi
  mkdir -p "$dest"
  printf 'FAKE-%s' "$to" > "$dest/$base.$ext"
  if echo "$input" | grep -q multi; then
    printf 'FAKE2' > "$dest/${base}-2.$ext"
  fi
  exit 0
fi

echo "fake-fbc: unknown invocation: $*" >&2
exit 2
