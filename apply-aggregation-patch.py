#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Nightingale 事件聚合功能补丁脚本（精简版）

聚合逻辑已全部落在独立文件 upstream/alert/dispatch/aggregation_engine.go 中，
本脚本只需将 dispatch.go 中「直接发送」的单行调用替换为「进入聚合引擎」，
从而将改动面缩到最小，最大化跟随上游升级的能力。

原始（上游）调用：
    go SendByNotifyRule(e.ctx, e.userCache, e.userGroupCache, e.notifyChannelCache,
        e.configCvalCache, []*models.AlertCurEvent{eventCopy}, notifyRuleId,
        &notifyRule.NotifyConfigs[i], notifyChannel, messageTemplate)

替换为：
    e.AddToAggregation(eventCopy, notifyRuleId, &notifyRule.NotifyConfigs[i],
        notifyChannel, messageTemplate)
"""

import os
import sys
import re

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))

if len(sys.argv) > 1:
    TARGET_FILE = sys.argv[1]
elif os.environ.get('TARGET_FILE'):
    TARGET_FILE = os.environ.get('TARGET_FILE')
else:
    TARGET_FILE = os.path.join(SCRIPT_DIR, "upstream/alert/dispatch/dispatch.go")

BACKUP_FILE = TARGET_FILE + ".bak"

# 上游原始的直接发送调用（精确匹配，允许中间空白差异）
SEND_PATTERN = (
    r'go SendByNotifyRule\(e\.ctx, e\.userCache, e\.userGroupCache, '
    r'e\.notifyChannelCache, e\.configCvalCache, '
    r'\[\]\*models\.AlertCurEvent\{eventCopy\}, notifyRuleId, '
    r'&notifyRule\.NotifyConfigs\[i\], notifyChannel, messageTemplate\)'
)

# 替换为聚合引擎入口（AddToAggregation 定义在 aggregation_engine.go）
REPLACEMENT = (
    '// 事件聚合：交给聚合引擎按配置窗口批量发送（见 aggregation_engine.go）\n'
    '\t\t\t\te.AddToAggregation(eventCopy, notifyRuleId, '
    '&notifyRule.NotifyConfigs[i], notifyChannel, messageTemplate)'
)

# 幂等标记：已应用则跳过
APPLIED_MARKER = 'e.AddToAggregation(eventCopy, notifyRuleId,'


def apply_patch():
    print("🚀 开始应用事件聚合功能补丁（精简版）...")
    print(f"🎯 目标文件: {TARGET_FILE}\n")

    if not os.path.exists(TARGET_FILE):
        print(f"❌ 错误: 目标文件不存在: {TARGET_FILE}")
        return False

    with open(TARGET_FILE, 'r', encoding='utf-8') as f:
        content = f.read()

    # 幂等检查
    if APPLIED_MARKER in content:
        print("✅ 补丁已存在，跳过应用")
        return True

    if not re.search(SEND_PATTERN, content):
        print("❌ 未找到目标发送调用，上游代码可能已变更，请人工核对 dispatch.go")
        return False

    # 备份
    with open(BACKUP_FILE, 'w', encoding='utf-8') as f:
        f.write(content)

    print("🔧 应用补丁（单行替换）...")
    content = re.sub(SEND_PATTERN, REPLACEMENT, content, count=1)

    with open(TARGET_FILE, 'w', encoding='utf-8') as f:
        f.write(content)

    with open(TARGET_FILE, 'r', encoding='utf-8') as f:
        new_content = f.read()

    if APPLIED_MARKER in new_content:
        print("✅ 补丁应用成功！\n")
        print("📊 补丁统计：")
        print("  - 修改发送逻辑: SendByNotifyRule -> AddToAggregation（1 行）")
        print("  - 聚合引擎实现: upstream/alert/dispatch/aggregation_engine.go")
        os.remove(BACKUP_FILE)
        return True
    else:
        print("❌ 补丁应用失败，恢复原文件")
        os.rename(BACKUP_FILE, TARGET_FILE)
        return False


if __name__ == '__main__':
    success = apply_patch()
    sys.exit(0 if success else 1)
