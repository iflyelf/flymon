#!/usr/bin/env bash
# flymon 构建脚本
# 支持 CGO_ENABLED=0 静态交叉编译，产出 linux/amd64 和 linux/arm64 二进制

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# 从 upstream 子模块读取版本号
UPSTREAM_VERSION=$(cd upstream && git describe --tags --exact-match 2>/dev/null || echo "unknown")
VERSION="${UPSTREAM_VERSION}-flymon"

# 默认平台
PLATFORMS="${PLATFORMS:-linux/amd64 linux/arm64}"

# 构建参数
LDFLAGS="-w -s -X github.com/ccfos/nightingale/v6/pkg/version.Version=${VERSION}"

echo "🚀 开始构建 flymon"
echo "📌 版本: ${VERSION}"
echo "🏗️  平台: ${PLATFORMS}"
echo ""

# 应用事件聚合补丁（精简版，单行替换）
if ! grep -q "AddToAggregation(eventCopy, notifyRuleId" upstream/alert/dispatch/dispatch.go; then
    echo "📦 应用事件聚合补丁..."
    python3 apply-aggregation-patch.py upstream/alert/dispatch/dispatch.go
    echo "✅ 补丁应用成功"
else
    echo "✅ 事件聚合补丁已存在"
fi
echo ""

# 清理旧产物
rm -rf bin dist
mkdir -p bin dist

# 构建函数
build_binary() {
    local os=$1
    local arch=$2
    local name=$3
    local cmd_path=$4
    
    local output_name="${name}"
    local dist_name="${name}-${os}-${arch}"
    
    if [[ "$os" == "linux" ]] && [[ "$arch" == "amd64" ]]; then
        # 默认平台输出到 bin/ 用于本地测试
        output_dir="bin"
    else
        output_dir="dist"
    fi
    
    echo "🔨 编译 ${dist_name}..."
    
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -trimpath \
        -ldflags "$LDFLAGS" \
        -o "${output_dir}/${output_name}" \
        "${cmd_path}"
    
    # 发布产物统一放 dist/
    if [[ "$os" == "linux" ]] && [[ "$arch" == "amd64" ]]; then
        cp "${output_dir}/${output_name}" "dist/${dist_name}"
    else
        mv "${output_dir}/${output_name}" "dist/${dist_name}"
    fi
    
    echo "✅ ${dist_name} 编译完成"
}

# 遍历平台编译三个服务
for platform in $PLATFORMS; do
    os="${platform%/*}"
    arch="${platform#*/}"
    
    build_binary "$os" "$arch" "flymon" "./cmd/flymon"
    build_binary "$os" "$arch" "flymon-edge" "./cmd/flymon-edge"
    build_binary "$os" "$arch" "flymon-pushgw" "./cmd/flymon-pushgw"
    build_binary "$os" "$arch" "flymon-gateway" "./cmd/flymon-gateway"
done

echo ""
echo "📊 构建产物:"
ls -lh dist/
echo ""
echo "🎉 构建完成！版本: ${VERSION}"
