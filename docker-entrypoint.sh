#!/bin/bash
set -e

# Flymon 启动脚本

echo "🚀 启动 Flymon (go-zero 事件聚合版)"
echo "📦 版本信息: $(cat /opt/flymon/etc/version.txt 2>/dev/null || echo 'unknown')"
echo "⏰ 时区: ${TZ}"
echo ""

# 等待依赖服务（如果配置了）
if [ -n "$WAIT_FOR" ]; then
    echo "⏳ 等待依赖服务: $WAIT_FOR"
    for service in $WAIT_FOR; do
        host="${service%%:*}"
        port="${service##*:}"
        timeout=60
        while ! nc -z "$host" "$port" 2>/dev/null; do
            timeout=$((timeout - 1))
            if [ $timeout -le 0 ]; then
                echo "❌ 等待 $service 超时"
                exit 1
            fi
            sleep 1
        done
        echo "✅ $service 已就绪"
    done
fi

# 根据命令启动不同组件
case "$1" in
    flymon)
        echo "🔧 启动 flymon (核心服务 + 事件聚合)"
        exec /opt/flymon/flymon --configs=/opt/flymon/etc
        ;;
    flymon-edge)
        echo "🌐 启动 flymon-edge (边缘节点 + 推送网关 + 告警引擎)"
        exec /opt/flymon/flymon-edge --configs=/opt/flymon/etc/edge
        ;;
    flymon-pushgw)
        echo "📤 启动 flymon-pushgw (推送网关)"
        exec /opt/flymon/flymon-pushgw --configs=/opt/flymon/etc
        ;;
    *)
        echo "📋 可用命令:"
        echo "  - flymon         (核心服务，含事件聚合)"
        echo "  - flymon-edge    (边缘节点，含推送网关+告警引擎)"
        echo "  - flymon-pushgw  (推送网关)"
        echo ""
        exec "$@"
        ;;
esac
