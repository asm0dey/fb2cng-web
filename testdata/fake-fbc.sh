#!/bin/sh
# Fake fbc for tests. Emulates: `dumpconfig --default` and
# `[-c cfg] convert --to FMT [--overwrite --nd] INPUT DEST`.
# convert emits a multi-line stdout+stderr log. A "corrupt" input fails with an
# ERR line unless the -c config contains `use_broken_images: true` (retry path).
# An input carrying the __hang marker in its name sleeps 3s before continuing,
# to exercise per-file conversion timeouts (bean ckkk).
mode=""
for a in "$@"; do
  case "$a" in
    dumpconfig) mode=dump ;;
    convert) mode=convert ;;
    *) ;;
  esac
done

if [ "$mode" = "dump" ]; then
  # Handle both `dumpconfig --default <out>` and `-c <cfg> dumpconfig <out>`.
  cfg=""
  dest_file=""
  while [ $# -gt 0 ]; do
    case "$1" in
      -c) cfg="$2"; shift 2; continue ;;
      dumpconfig|--default) shift; continue ;;
      *) dest_file="$1"; shift ;;
    esac
  done
  # Validation probe: reject a config carrying the __invalid marker.
  if [ -n "$cfg" ] && grep -q '__invalid' "$cfg" 2>/dev/null; then
    echo "fake-fbc: invalid config: $cfg" >&2
    exit 1
  fi
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

  # HANG mode: an input whose name carries the __hang marker sleeps past any
  # sane per-file timeout, so tests can assert the worker kills it instead of
  # waiting out the full sleep. `exec` replaces this shell with sleep itself
  # (rather than forking a child) so a ctx-cancel SIGKILL on this process
  # doesn't leave an orphaned grandchild holding the caller's stdout/stderr
  # pipe open, which would otherwise stall the caller's cmd.Wait() for the
  # full sleep regardless of the timeout.
  if echo "$input" | grep -q '__hang'; then
    exec sleep 3
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
