package gateway

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// 飞书 IM 客户端：获取 token、推送交互卡片
// ============================================================================

type FeishuClient struct {
	domain string
	client *http.Client
}

func NewFeishuClient(cfg *Config) *FeishuClient {
	return &FeishuClient{
		domain: cfg.FeishuDomain,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// GetAccessToken 获取 tenant_access_token
func (f *FeishuClient) GetAccessToken(appID, appSecret string) (string, error) {
	url := fmt.Sprintf("https://%s/open-apis/auth/v3/tenant_access_token/internal", f.domain)
	payload := map[string]string{"app_id": appID, "app_secret": appSecret}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if code, ok := result["code"].(float64); ok && code != 0 {
		return "", fmt.Errorf("获取飞书 token 失败: %v", result["msg"])
	}
	token, _ := result["tenant_access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("飞书 token 为空")
	}
	return token, nil
}

// SendCard 向指定接收者推送交互卡片
func (f *FeishuClient) SendCard(feishuToken, receiveID, receiveIDType string, card map[string]interface{}) (string, error) {
	url := fmt.Sprintf("https://%s/open-apis/im/v1/messages", f.domain)
	contentBytes, _ := json.Marshal(card)
	payload := map[string]string{
		"receive_id": receiveID,
		"msg_type":   "interactive",
		"content":    string(contentBytes),
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+feishuToken)
	req.Header.Set("Content-Type", "application/json")
	q := req.URL.Query()
	q.Set("receive_id_type", receiveIDType)
	req.URL.RawQuery = q.Encode()

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if code, ok := result["code"].(float64); ok && code != 0 {
		return "", fmt.Errorf("发送飞书卡片失败: %v", result["msg"])
	}

	// 返回 message_id
	if data, ok := result["data"].(map[string]interface{}); ok {
		if mid, ok := data["message_id"].(string); ok {
			return mid, nil
		}
	}
	return "", nil
}

// doJSON 发送 JSON 请求并解析响应为 map，附带 query 参数。
func (f *FeishuClient) doJSON(method, url, feishuToken string, query map[string]string, payload interface{}) (map[string]interface{}, error) {
	var reader io.Reader
	if payload != nil {
		body, _ := json.Marshal(payload)
		reader = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, reader)
	req.Header.Set("Authorization", "Bearer "+feishuToken)
	req.Header.Set("Content-Type", "application/json")
	if len(query) > 0 {
		q := req.URL.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if len(respBody) > 0 {
		json.Unmarshal(respBody, &result)
	}
	if result == nil {
		result = map[string]interface{}{}
	}
	return result, nil
}

// CreateChat 创建飞书群聊，点击者作为初始成员，机器人自动成为群主。
// 返回 (chatID, 成功, 错误消息)。
func (f *FeishuClient) CreateChat(feishuToken, chatName, ownerOpenID string) (string, bool, string) {
	url := fmt.Sprintf("https://%s/open-apis/im/v1/chats", f.domain)
	payload := map[string]interface{}{
		"name":                       chatName,
		"description":                "告警协同群，由信息化监控系统自动创建",
		"user_id_list":               []string{ownerOpenID},
		"set_bot_manager":            true,
		"chat_mode":                  "group",
		"chat_type":                  "private",
		"join_message_visibility":    "only_owner",
		"leave_message_visibility":   "only_owner",
		"membership_approval":        "no_approval_required",
	}
	result, err := f.doJSON("POST", url, feishuToken, map[string]string{"user_id_type": "open_id"}, payload)
	if err != nil {
		return "", false, fmt.Sprintf("创建异常: %v", err)
	}
	if code, _ := result["code"].(float64); code != 0 {
		return "", false, fmt.Sprintf("创建失败: %v", result["msg"])
	}
	chatID := ""
	if data, ok := result["data"].(map[string]interface{}); ok {
		chatID, _ = data["chat_id"].(string)
	}
	if chatID == "" {
		return "", false, "创建失败: 未返回 chat_id"
	}
	return chatID, true, "群聊创建成功"
}

// BatchGetOpenIDByEmail 通过邮箱批量查询 open_id，返回 email->open_id 映射。
func (f *FeishuClient) BatchGetOpenIDByEmail(feishuToken string, emails []string) map[string]string {
	mapping := make(map[string]string)
	if len(emails) == 0 {
		return mapping
	}
	url := fmt.Sprintf("https://%s/open-apis/contact/v3/users/batch_get_id", f.domain)
	result, err := f.doJSON("POST", url, feishuToken,
		map[string]string{"user_id_type": "open_id"},
		map[string]interface{}{"emails": emails})
	if err != nil {
		logger.Warningf("邮箱转 open_id 异常: %v", err)
		return mapping
	}
	if code, _ := result["code"].(float64); code != 0 {
		logger.Warningf("邮箱转 open_id 失败: %v", result["msg"])
		return mapping
	}
	if data, ok := result["data"].(map[string]interface{}); ok {
		if userList, ok := data["user_list"].([]interface{}); ok {
			for _, item := range userList {
				if u, ok := item.(map[string]interface{}); ok {
					email, _ := u["email"].(string)
					userID, _ := u["user_id"].(string)
					if email != "" && userID != "" {
						mapping[email] = userID
					}
				}
			}
		}
	}
	return mapping
}

// AddMembersByOpenID 通过 open_id 向群聊添加成员。
func (f *FeishuClient) AddMembersByOpenID(feishuToken, chatID string, openIDs []string) bool {
	if len(openIDs) == 0 {
		return true
	}
	url := fmt.Sprintf("https://%s/open-apis/im/v1/chats/%s/members", f.domain, chatID)
	result, err := f.doJSON("POST", url, feishuToken,
		map[string]string{"member_id_type": "open_id"},
		map[string]interface{}{"id_list": openIDs})
	if err != nil {
		logger.Warningf("群成员添加异常: %v", err)
		return false
	}
	if code, _ := result["code"].(float64); code != 0 {
		logger.Warningf("群成员添加失败: %v", result["msg"])
		return false
	}
	return true
}

// AddMembersByEmail 通过邮箱向群聊添加成员，返回添加失败的邮箱列表。
func (f *FeishuClient) AddMembersByEmail(feishuToken, chatID string, emails []string) []string {
	if len(emails) == 0 {
		return nil
	}
	mapping := f.BatchGetOpenIDByEmail(feishuToken, emails)
	if len(mapping) == 0 {
		return emails
	}
	var invalid []string
	for _, e := range emails {
		if _, ok := mapping[e]; !ok {
			invalid = append(invalid, e)
		}
	}
	var openIDs []string
	for _, id := range mapping {
		openIDs = append(openIDs, id)
	}
	if !f.AddMembersByOpenID(feishuToken, chatID, openIDs) {
		return emails
	}
	return invalid
}

// AddChatManagers 添加群管理员（支持 email / open_id 混合）。
func (f *FeishuClient) AddChatManagers(feishuToken, chatID string, managerIDs []string) bool {
	if len(managerIDs) == 0 {
		return true
	}
	url := fmt.Sprintf("https://%s/open-apis/im/v1/chats/%s/managers/add_managers", f.domain, chatID)
	var emailIDs, openIDs []string
	for _, mid := range managerIDs {
		if strings.Contains(mid, "@") {
			emailIDs = append(emailIDs, mid)
		} else {
			openIDs = append(openIDs, mid)
		}
	}
	anySuccess := false

	if len(openIDs) > 0 {
		result, err := f.doJSON("POST", url, feishuToken,
			map[string]string{"member_id_type": "open_id"},
			map[string]interface{}{"manager_ids": openIDs})
		if err == nil {
			if code, _ := result["code"].(float64); code == 0 {
				anySuccess = true
			} else {
				logger.Warningf("管理员添加失败(open_id): %v", result["msg"])
			}
		}
	}

	if len(emailIDs) > 0 {
		mapping := f.BatchGetOpenIDByEmail(feishuToken, emailIDs)
		var convertedIDs []string
		for _, id := range mapping {
			convertedIDs = append(convertedIDs, id)
		}
		if len(convertedIDs) > 0 {
			// 先加为成员再提升为管理员
			f.AddMembersByOpenID(feishuToken, chatID, convertedIDs)
			result, err := f.doJSON("POST", url, feishuToken,
				map[string]string{"member_id_type": "open_id"},
				map[string]interface{}{"manager_ids": convertedIDs})
			if err == nil {
				if code, _ := result["code"].(float64); code == 0 {
					anySuccess = true
				} else {
					logger.Warningf("管理员添加失败(email->open_id): %v", result["msg"])
				}
			}
		} else {
			logger.Warningf("邮箱转 open_id 失败，无法添加管理员: %v", emailIDs)
		}
	}
	return anySuccess
}

// DismissChat 解散飞书群聊。
func (f *FeishuClient) DismissChat(feishuToken, chatID string) (bool, string) {
	url := fmt.Sprintf("https://%s/open-apis/im/v1/chats/%s", f.domain, chatID)
	result, err := f.doJSON("DELETE", url, feishuToken, nil, nil)
	if err != nil {
		return false, fmt.Sprintf("解散异常: %v", err)
	}
	code, _ := result["code"].(float64)
	if code == 0 {
		return true, "群聊解散成功"
	}
	msg, _ := result["msg"].(string)
	if strings.Contains(strings.ToLower(msg), "dissolved") || strings.Contains(msg, "已解散") {
		return true, "群聊已解散"
	}
	return false, fmt.Sprintf("解散失败: %s", msg)
}

// SendCardToChat 向群聊发送交互卡片。
func (f *FeishuClient) SendCardToChat(feishuToken, chatID string, card map[string]interface{}) bool {
	contentBytes, _ := json.Marshal(card)
	url := fmt.Sprintf("https://%s/open-apis/im/v1/messages", f.domain)
	result, err := f.doJSON("POST", url, feishuToken,
		map[string]string{"receive_id_type": "chat_id"},
		map[string]interface{}{
			"receive_id": chatID,
			"msg_type":   "interactive",
			"content":    string(contentBytes),
		})
	if err != nil {
		logger.Errorf("群卡片发送异常: %v", err)
		return false
	}
	if code, _ := result["code"].(float64); code != 0 {
		logger.Errorf("群卡片发送失败: %v", result["msg"])
		return false
	}
	return true
}

// PatchCard 更新已发送的卡片内容（message update）
func (f *FeishuClient) PatchCard(feishuToken, messageID string, card map[string]interface{}) error {
	url := fmt.Sprintf("https://%s/open-apis/im/v1/messages/%s", f.domain, messageID)
	contentBytes, _ := json.Marshal(card)
	payload := map[string]string{"content": string(contentBytes)}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+feishuToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
	return nil
}
