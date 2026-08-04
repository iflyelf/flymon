package gateway

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// N9E API 客户端：创建/删除屏蔽规则，带重试
// ============================================================================

type N9EClient struct {
	baseURL  string
	token    string
	client   *http.Client
	retryCfg RetryConfig
}

type RetryConfig struct {
	MaxAttempts   int
	DelayBase     int
	DelayMax      int
	BackoffFactor int
}

func NewN9EClient(cfg *Config) *N9EClient {
	return &N9EClient{
		baseURL: cfg.N9EAPIURL,
		token:   cfg.N9EAPIToken,
		client: &http.Client{
			Timeout: time.Duration(cfg.APITimeout) * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		retryCfg: RetryConfig{
			MaxAttempts:   cfg.RetryMaxAttempts,
			DelayBase:     cfg.RetryDelayBase,
			DelayMax:      cfg.RetryDelayMax,
			BackoffFactor: cfg.RetryBackoffFactor,
		},
	}
}

// CreateMuteRule 创建单条屏蔽规则（带重试）
func (c *N9EClient) CreateMuteRule(groupID int, note string, tags []map[string]interface{}, muteTimeType int, btime, etime int64) (bool, string) {
	url := fmt.Sprintf("%s/busi-group/%d/alert-mutes", c.baseURL, groupID)
	payload := map[string]interface{}{
		"group_id":       groupID,
		"note":           note,
		"cate":           "prometheus",
		"datasource_ids": []int{0},
		"severities":     []int{1, 2, 3},
		"tags":           tags,
		"mute_time_type": muteTimeType,
		"btime":          btime,
		"etime":          etime,
		"periodic_mutes": []map[string]interface{}{
			{
				"enable_days_of_week": "1 2 3 4 5 6 0",
				"enable_stime":        "00:00",
				"enable_etime":        func() string { if muteTimeType == 1 { return "23:59" }; return "00:00" }(),
			},
		},
		"cluster": "0",
	}

	return c.doWithRetry("POST", url, payload)
}

// ListMuteRules 查询业务组下所有屏蔽规则
func (c *N9EClient) ListMuteRules(groupID int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/busi-group/%d/alert-mutes", c.baseURL, groupID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-User-Token", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("列表接口失败: status=%d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if errMsg, ok := result["err"].(string); ok && errMsg != "" {
		return nil, fmt.Errorf("API 错误: %s", errMsg)
	}

	dat, ok := result["dat"].([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	rules := make([]map[string]interface{}, 0, len(dat))
	for _, item := range dat {
		if r, ok := item.(map[string]interface{}); ok {
			rules = append(rules, r)
		}
	}
	return rules, nil
}

// DeleteMuteRule 删除单条屏蔽规则
func (c *N9EClient) DeleteMuteRule(groupID int, ruleID int) (bool, string) {
	url := fmt.Sprintf("%s/busi-group/%d/alert-mutes", c.baseURL, groupID)
	payload := map[string]interface{}{"ids": []int{ruleID}}
	return c.doWithRetry("DELETE", url, payload)
}

func (c *N9EClient) doWithRetry(method, url string, payload map[string]interface{}) (bool, string) {
	var lastErr string
	for attempt := 0; attempt < c.retryCfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := c.retryCfg.DelayBase
			for i := 1; i < attempt; i++ {
				delay *= c.retryCfg.BackoffFactor
				if delay > c.retryCfg.DelayMax {
					delay = c.retryCfg.DelayMax
					break
				}
			}
			logger.Warningf("N9E API 第 %d 次重试，延迟 %d 秒", attempt, delay)
			time.Sleep(time.Duration(delay) * time.Second)
		}

		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest(method, url, bytes.NewReader(body))
		req.Header.Set("X-User-Token", c.token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 200 {
			var result map[string]interface{}
			if err := json.Unmarshal(respBody, &result); err == nil {
				if errMsg, ok := result["err"].(string); ok && errMsg != "" {
					return false, errMsg
				}
			}
			return true, "success"
		}

		lastErr = fmt.Sprintf("status=%d, body=%s", resp.StatusCode, string(respBody))
		if resp.StatusCode >= 500 {
			// 5xx 重试
			continue
		}
		// 4xx 不重试
		return false, lastErr
	}

	return false, fmt.Sprintf("重试 %d 次后仍失败: %s", c.retryCfg.MaxAttempts, lastErr)
}
