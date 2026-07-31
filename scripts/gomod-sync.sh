#!/usr/bin/env bash
# flymon 依赖同步脚本
#
# 用途：当 upstream 子模块升级到新版本后，同步 flymon 主模块的依赖。
#
# 背景：Go modules 不会继承依赖模块（upstream）的 replace 指令，
# 而 upstream（夜莺）依赖若干 fork 版本（如 x/exp、n9e/elastic）。
# 直接对 flymon 执行 `go mod tidy` 会导致：
#   1. upstream 的 replace 未被继承，解析到不兼容版本（编译失败）
#   2. go 1.26+ 的 MVS 升级 x/exp，破坏 prometheus 旧版兼容性
#
# 因此本脚本自动从 upstream/go.mod 提取所有 replace 并合并到 flymon/go.mod。

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "🔍 从 upstream/go.mod 提取 replace 指令..."

# 提取 upstream 的 replace（排除本地路径 replace 和注释）
UPSTREAM_REPLACES=$(grep -E "^replace " upstream/go.mod | grep -v "=> \.\./" || true)

echo "📦 upstream replace 指令:"
echo "$UPSTREAM_REPLACES"
echo ""

echo "🛠️  执行 go mod tidy（保留 upstream replace）..."
# 先 tidy 拉取依赖
go mod tidy || true

# 重新确保 upstream replace 存在（tidy 可能移除）
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    # 提取被 replace 的模块名（replace A => B 中的 A）
    mod=$(echo "$line" | sed -E 's/^replace ([^ ]+).*/\1/')
    if ! grep -qE "^replace ${mod//\//\\/} " go.mod; then
        echo "➕ 补充缺失的 replace: $line"
        echo "$line" >> go.mod
    fi
done <<< "$UPSTREAM_REPLACES"

echo ""
echo "✅ 依赖同步完成。请执行 scripts/build.sh 验证编译。"
