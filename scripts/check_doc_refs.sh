#!/usr/bin/env bash
# 校验 Markdown 文档中的 file:line 引用（防止文档漂移）
#
# 用法:
#   bash scripts/check_doc_refs.sh                 # 扫描仓库 docs/ 与根目录 md
#   bash scripts/check_doc_refs.sh <root>          # 指定扫描根目录
#
# 规则: 识别 `<path>:<line>`（如 docs/architecture.md:12、apps/foo.go:34），
# 文件必须存在且行号不超过文件总行数。
set -uo pipefail

ROOT="${1:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
fail=0

while IFS= read -r doc; do
    while IFS=: read -r file line; do
        [ -z "$file" ] && continue
        [ -z "$line" ] && continue
        path="${ROOT}/${file}"
        if [ ! -f "$path" ]; then
            echo "MISSING FILE: $doc -> $file:$line"
            fail=1
            continue
        fi
        total=$(wc -l < "$path" | tr -d ' ')
        if [ "$line" -gt "$total" ]; then
            echo "LINE OUT OF RANGE: $doc -> $file:$line (file has $total lines)"
            fail=1
        fi
    done < <(grep -oE '[A-Za-z0-9_./-]+\.(go|ts|tsx|js|jsx|py|sh|yaml|yml|sql|proto):[0-9]+' "$doc" | sed -E 's/:([0-9]+)$/:\1/')
done < <(find "$ROOT" -maxdepth 2 \( -path '*/node_modules' -o -path '*/.git' \) -prune -o -name '*.md' -type f -print)

if [ "$fail" -ne 0 ]; then
    echo "文档引用校验失败，请更新 file:line 引用" >&2
    exit 1
fi
echo "文档引用校验通过"
