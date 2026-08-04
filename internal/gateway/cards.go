package gateway

import (
	"fmt"
)

// ============================================================================
// 飞书卡片渲染：告警卡片、AI 结果卡片、简单通知卡片、HTML toast
// ============================================================================

// buildAIResultCard 构建 AI 分析结果卡片。
func buildAIResultCard(aiData map[string]interface{}, result AIResult) map[string]interface{} {
	ruleName := getString(aiData, "rule_name")
	header := fmt.Sprintf("🤖 AI 告警分析 - %s", ruleName)
	template := "blue"
	if !result.Success {
		template = "red"
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag": "div",
			"text": map[string]string{
				"tag": "lark_md",
				"content": fmt.Sprintf(
					"**规则:** %s  \n**级别:** S%v  \n**主机:** %v  \n**实例:** %v  \n**时间:** %v",
					ruleName, aiData["severity"], aiData["hosts"], aiData["instance"], aiData["time"],
				),
			},
		},
		map[string]interface{}{"tag": "hr"},
	}

	if result.Success {
		elements = append(elements,
			map[string]interface{}{
				"tag":  "div",
				"text": map[string]string{"tag": "lark_md", "content": "**🔍 根因分析**\n\n" + result.RootCause},
			},
			map[string]interface{}{"tag": "hr"},
			map[string]interface{}{
				"tag":  "div",
				"text": map[string]string{"tag": "lark_md", "content": "**💡 处理建议**\n\n" + result.Suggestion},
			},
			map[string]interface{}{"tag": "hr"},
			map[string]interface{}{
				"tag":  "div",
				"text": map[string]string{"tag": "lark_md", "content": "**⚠️ 影响评估**\n\n" + result.Impact},
			},
			map[string]interface{}{"tag": "hr"},
			map[string]interface{}{
				"tag":  "div",
				"text": map[string]string{"tag": "lark_md", "content": "*分析由 iflyelf 专属AI 生成，仅供参考。*"},
			},
		)
	} else {
		elements = append(elements,
			map[string]interface{}{
				"tag":  "div",
				"text": map[string]string{"tag": "lark_md", "content": fmt.Sprintf("**❌ 分析失败**\n\n%s", result.Error)},
			},
		)
	}

	return map[string]interface{}{
		"config": map[string]bool{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]string{"tag": "plain_text", "content": header},
			"template": template,
		},
		"elements": elements,
	}
}

// renderAIToast 渲染 AI 分析触发端点的 HTML 提示页面。
func renderAIToast(status, message string) string {
	colorMap := map[string]string{
		"success": "#1890ff",
		"info":    "#1890ff",
		"error":   "#f5222d",
	}
	color := colorMap[status]
	if color == "" {
		color = "#1890ff"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><title>AI 分析</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: -apple-system, 'PingFang SC', sans-serif;
  background: linear-gradient(135deg, #f5f7fa 0%%, #e8ecf3 100%%);
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}
.box {
  background: #fff;
  border-radius: 16px;
  padding: 40px 32px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08);
  text-align: center;
  max-width: 360px;
  width: 100%%;
}
.msg { color: %s; font-size: 18px; font-weight: 600; margin-bottom: 16px; }
.tip { color: #999; font-size: 13px; }
.countdown { color: #ccc; font-size: 12px; margin-top: 20px; }
</style></head>
<body>
<div class="box">
  <div class="msg">%s</div>
  <div class="tip">分析结果将在飞书群中推送</div>
  <div class="countdown">页面将在 <span id="sec">3</span> 秒后自动关闭</div>
</div>
<script>
var sec = 3;
setInterval(function(){ sec--; document.getElementById('sec').textContent=sec; if(sec<=0){window.close();} }, 1000);
</script>
</body>
</html>`, color, message)
}

// buildSimpleNotifyCard 构建简单通知卡片（用于群聊创建/解散结果通知）。
func buildSimpleNotifyCard(title, content, color string) map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]bool{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]string{"tag": "plain_text", "content": title},
			"template": color,
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag":  "div",
				"text": map[string]string{"tag": "lark_md", "content": content},
			},
		},
	}
}

// buildAlertCardForGroup 根据告警卡片快照构建发送到新群的卡片（移除交互按钮）。
func buildAlertCardForGroup(snapshot map[string]interface{}) map[string]interface{} {
	headerText := getString(snapshot, "header_text")
	if headerText == "" {
		headerText = "信息化监控告警"
	}
	template := getString(snapshot, "template")
	if template == "" {
		template = "red"
	}
	markdown := getString(snapshot, "markdown")
	imageKey := getString(snapshot, "image_key")

	var elements []interface{}
	if imageKey != "" {
		elements = append(elements, map[string]interface{}{
			"tag":     "img",
			"img_key": imageKey,
			"alt":     map[string]string{"tag": "plain_text", "content": "监控图表"},
		})
	}
	if markdown != "" {
		elements = append(elements, map[string]interface{}{
			"tag":     "markdown",
			"content": markdown,
		})
	}
	elements = append(elements, map[string]interface{}{"tag": "hr"})
	elements = append(elements, map[string]interface{}{
		"tag":     "markdown",
		"content": "*本群由告警协同功能自动创建，用于快速响应和处理告警事件。*",
	})

	return map[string]interface{}{
		"config": map[string]bool{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]string{"tag": "plain_text", "content": headerText},
			"template": template,
		},
		"elements": elements,
	}
}
