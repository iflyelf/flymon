package gateway

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// AI 分析引擎：多提供商调度（OpenClaw / iflyelf），支持故障转移
// ============================================================================

type AIProvider struct {
	Name  string
	URL   string
	Token string
	Model string
}

type AIResult struct {
	Success    bool
	Provider   string
	Fallback   bool
	RootCause  string
	Suggestion string
	Impact     string
	Raw        string
	Error      string
}

type AIEngine struct {
	cfg    *Config
	client *http.Client
}

func NewAIEngine(cfg *Config) *AIEngine {
	return &AIEngine{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.AITimeout) * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// Analyze 执行 AI 告警分析（多提供商故障转移）
func (a *AIEngine) Analyze(aiData map[string]interface{}) AIResult {
	prompt := a.buildPrompt(aiData)
	chain := a.resolveProviderChain()

	if len(chain) == 0 {
		return AIResult{Success: false, Error: "AI 服务全部禁用"}
	}

	var errors []string
	for i, p := range chain {
		logger.Infof("尝试 AI 提供商: %s (%d/%d)", p.Name, i+1, len(chain))
		ok, content := a.callChatCompletions(p, prompt)
		if ok {
			rc, sg, im := a.parseSections(content)
			return AIResult{
				Success:    true,
				Provider:   p.Name,
				Fallback:   i > 0,
				RootCause:  rc,
				Suggestion: sg,
				Impact:     im,
				Raw:        content,
			}
		}
		errors = append(errors, fmt.Sprintf("[%s] %s", p.Name, content))
		logger.Warningf("AI 提供商调用失败: %s - %s", p.Name, content)
	}

	return AIResult{Success: false, Error: "所有 AI 服务均失败: " + strings.Join(errors, " | ")}
}

func (a *AIEngine) buildPrompt(aiData map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString("你是资深运维告警分析专家。请基于以下告警信息，严格按 Markdown 三段式输出分析。\n\n")
	sb.WriteString("【告警信息】\n")
	sb.WriteString(fmt.Sprintf("- 规则名称: %v\n", aiData["rule_name"]))
	sb.WriteString(fmt.Sprintf("- 告警级别: S%v\n", aiData["severity"]))
	sb.WriteString(fmt.Sprintf("- 业务分组: %v\n", aiData["group_name"]))
	sb.WriteString(fmt.Sprintf("- 受影响主机: %v\n", aiData["hosts"]))
	sb.WriteString(fmt.Sprintf("- 实例: %v\n", aiData["instance"]))
	sb.WriteString(fmt.Sprintf("- 触发时间: %v\n", aiData["time"]))
	sb.WriteString(fmt.Sprintf("- 告警描述: %v\n", aiData["notes"]))
	sb.WriteString(fmt.Sprintf("- PromQL: %v\n", aiData["prom_ql"]))
	sb.WriteString("\n请严格按以下三段输出（使用 ## 标题）：\n\n")
	sb.WriteString("## 根因分析\n")
	sb.WriteString("(3-5 条可能的根因，覆盖资源/应用/网络/配置维度)\n\n")
	sb.WriteString("## 处理建议\n")
	sb.WriteString("(3-5 条可执行的排查/修复步骤，含具体 Linux 命令)\n\n")
	sb.WriteString("## 影响评估\n")
	sb.WriteString("(影响范围、严重程度、对业务的潜在影响，1-3 条)\n")
	return sb.String()
}

func (a *AIEngine) resolveProviderChain() []AIProvider {
	var openclaw, iflyelf *AIProvider
	if a.cfg.OpenClawEnabled {
		openclaw = &AIProvider{
			Name:  "openclaw",
			URL:   a.cfg.OpenClawAPIURL,
			Token: a.cfg.OpenClawAPIToken,
			Model: a.cfg.OpenClawModel,
		}
	}
	if a.cfg.IflyelfEnabled {
		iflyelf = &AIProvider{
			Name:  "iflyelf",
			URL:   a.cfg.IflyelfAPIURL,
			Token: a.cfg.IflyelfAPIToken,
			Model: a.cfg.IflyelfModel,
		}
	}

	switch a.cfg.AIProviderStrategy {
	case "openclaw_first":
		return filterNil([]*AIProvider{openclaw, iflyelf})
	case "iflyelf_first":
		return filterNil([]*AIProvider{iflyelf, openclaw})
	case "openclaw_only":
		return filterNil([]*AIProvider{openclaw})
	case "iflyelf_only":
		return filterNil([]*AIProvider{iflyelf})
	default:
		return filterNil([]*AIProvider{openclaw, iflyelf})
	}
}

func filterNil(ps []*AIProvider) []AIProvider {
	var result []AIProvider
	for _, p := range ps {
		if p != nil {
			result = append(result, *p)
		}
	}
	return result
}

func (a *AIEngine) callChatCompletions(p AIProvider, prompt string) (bool, string) {
	url := p.URL + "/v1/chat/completions"
	payload := map[string]interface{}{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是专业的运维告警分析专家，回答简洁、结构化、面向一线运维。"},
			{"role": "user", "content": prompt},
		},
		"stream":     false,
		"max_tokens": a.cfg.AIMaxTokens,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("连接失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		truncated := string(respBody)
		if len(truncated) > 120 {
			truncated = truncated[:120]
		}
		return false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncated)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false, "响应解析失败"
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return false, "choices 为空"
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return false, "choice 格式错误"
	}
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return false, "message 格式错误"
	}
	content, ok := message["content"].(string)
	if !ok || content == "" {
		return false, "content 为空"
	}

	return true, content
}

func (a *AIEngine) parseSections(content string) (string, string, string) {
	// 用 ## 标题切分三段
	re := regexp.MustCompile(`##\s*`)
	parts := re.Split(content, -1)

	var rc, sg, im string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "根因分析") {
			rc = strings.TrimSpace(strings.TrimPrefix(p, "根因分析"))
		} else if strings.HasPrefix(p, "处理建议") {
			sg = strings.TrimSpace(strings.TrimPrefix(p, "处理建议"))
		} else if strings.HasPrefix(p, "影响评估") {
			im = strings.TrimSpace(strings.TrimPrefix(p, "影响评估"))
		}
	}

	// 回退
	if rc == "" && sg == "" && im == "" {
		if len(content) > 1500 {
			rc = content[:1500]
		} else {
			rc = content
		}
		sg = "建议人工介入排查"
		im = "影响待评估"
	}

	return rc, sg, im
}
