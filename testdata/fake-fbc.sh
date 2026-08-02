#!/bin/sh
# Fake fbc for tests. Emulates: `dumpconfig --default` and
# `[-c cfg] convert --to FMT [--overwrite --nd] INPUT DEST`.
# convert emits a multi-line stdout+stderr log. A "corrupt" input fails with an
# ERR line unless the -c config contains `use_broken_images: true` (retry path).
mode=""
for a in "$@"; do
  case "$a" in
    dumpconfig) mode=dump ;;
    convert) mode=convert ;;
  esac
done

if [ "$mode" = "dump" ]; then
  dest_file=""
  for a in "$@"; do
    case "$a" in
      dumpconfig|--default) ;;
      *) dest_file="$a" ;;
    esac
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
  cfg=""
  positionals=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --to) to="$2"; shift 2; continue ;;
      -c) cfg="$2"; shift 2; continue ;;
      convert|--overwrite|--ow|--nd|--nodirs|-d|--debug) shift; continue ;;
      *) positionals="$positionals $1"; shift ;;
    esac
  done
  # positionals = INPUT ... DEST  (last is dest, first is input)
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

  broken=""
  if [ -n "$cfg" ] && grep -q 'use_broken_images: true' "$cfg" 2>/dev/null; then
    broken="yes"
  fi

  echo "INFO: opening $input"
  echo "INFO: target format $to" >&2
  if echo "$input" | grep -q corrupt && [ -z "$broken" ]; then
    echo "ERR: cannot parse $input: broken image" >&2
    echo "INFO: aborted"
    exit 1
  fi
  echo "INFO: converting $base"
  echo "WARN: cosmetic issue in $base" >&2
  mkdir -p "$dest"
  printf 'FAKE-%s' "$to" > "$dest/$base.$ext"
  if echo "$input" | grep -q multi; then
    printf 'FAKE2' > "$dest/${base}-2.$ext"
  fi
  echo "INFO: wrote $base.$ext"
  exit 0
fi

echo "fake-fbc: unknown invocation: $*" >&2
exit 2
