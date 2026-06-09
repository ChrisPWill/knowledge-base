#!/bin/sh

for arg in "$@"; do
  root="$arg"
done

cat <<EOF
{"type":"begin","data":{"path":{"text":"$root"}}}
{"type":"match","data":{"path":{"text":"$root/journals/2026_06_09.md"},"lines":{"text":"note #private #project/foo\n"},"line_number":12}}
{"type":"match","data":{"path":{"text":"$root/pages/Project.md"},"lines":{"text":"summary [[project/foo]]\n"},"line_number":4}}
{"type":"match","data":{"path":{"text":"$root/pages/Project.md"},"lines":{"text":"summary [[project/foo]]\n"},"line_number":4}}
{"type":"match","data":{"path":{"text":"$root/pages/Ops.md"},"lines":{"text":"rollout #ops+prod\n"},"line_number":3}}
{"type":"end","data":{"path":{"text":"$root"}}}
EOF
