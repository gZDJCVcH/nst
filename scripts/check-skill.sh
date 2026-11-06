#!/bin/sh
# 校验仓库自带的 Agent Skill 结构，本地与 CI 通用。
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
SKILL="$ROOT/skills/nst"
FILE="$SKILL/SKILL.md"

fail() {
	printf 'skill 校验失败：%s\n' "$*" >&2
	exit 1
}

[ -f "$FILE" ] || fail "缺少 $FILE"

name=$(sed -n 's/^name:[[:space:]]*//p' "$FILE" | head -n 1 | tr -d '\r')
[ "$name" = "nst" ] || fail "name 应为 nst，实际 '$name'"

desc=$(sed -n 's/^description:[[:space:]]*//p' "$FILE" | head -n 1)
[ -n "$desc" ] || fail "description 为空"

lines=$(wc -l < "$FILE" | tr -d ' ')
[ "$lines" -lt 500 ] || fail "SKILL.md 超过 500 行（$lines）"

refs=$(grep -oE 'references/[A-Za-z0-9._/-]+\.md' "$FILE" | sort -u || true)
for ref in $refs; do
	[ -f "$SKILL/$ref" ] || fail "主文档引用不存在：$ref"
done

nested=$(find "$SKILL/references" -name SKILL.md -print)
[ -z "$nested" ] || fail "references 下不应再出现 SKILL.md：$nested"

for doc in $(find "$SKILL/references" -name '*.md' -type f); do
	dir=$(dirname "$doc")
	local_refs=$(grep -oE '[A-Za-z0-9._-]+\.md' "$doc" | sort -u || true)
	for ref in $local_refs; do
		[ "$ref" = "SKILL.md" ] && continue
		[ -f "$dir/$ref" ] || fail "$doc 引用不存在：$ref"
	done
done

printf '✓ skill 结构校验通过\n'
