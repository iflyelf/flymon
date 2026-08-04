package gateway

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// HTTP 处理器：飞书回调、token 注册、健康检查
// ============================================================================

// dedupCache 线程安全的去重缓存（原全局变量改为结构体字段）。
type dedupCache struct {
	mu    sync.Mutex
	items map[string]int64
	ttl   int64
}

func newDedupCache(ttl int64) *dedupCache {
	return &dedupCache{items: make(map[string]int64), ttl: ttl}
}

func (d *dedupCache) isDuplicate(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().Unix()
	// 清理过期
	for k, ts := range d.items {
		if now-ts > d.ttl {
			delete(d.items, k)
		}
	}

	if _, exists := d.items[key]; exists {
		return true
	}
	d.items[key] = now
	return false
}

// Server 持有 gateway 各类依赖与线程安全去重缓存。
type Server struct {
	cfg    *Config
	n9e    *N9EClient
	feishu *FeishuClient
	ai     *AIEngine

	dedup   *dedupCache // 飞书回调去重（5 分钟）
	aiDedup *dedupCache // AI 分析去重（60 秒）
	gcDedup *dedupCache // 协同群去重（60 秒）
}

// NewServer 创建 gateway Server。
func NewServer(cfg *Config, n9e *N9EClient, fs *FeishuClient, ai *AIEngine) *Server {
	return &Server{
		cfg:     cfg,
		n9e:     n9e,
		feishu:  fs,
		ai:      ai,
		dedup:   newDedupCache(300),
		aiDedup: newDedupCache(60),
		gcDedup: newDedupCache(60),
	}
}

// ============================================================================
// / - 服务状态
// ============================================================================

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"service": "nightingale-mute-callback",
		"status":  "running",
		"version": "2.0.0-go",
		"endpoints": []string{
			"POST /feishu_callback - 飞书卡片交互回调（屏蔽 + AI 分析 + 协同群）",
			"POST /mute/register - 注册屏蔽参数 token",
			"POST /ai_analysis/register - 注册 AI 分析参数 token",
			"POST /group_chat/register - 注册一键拉群参数 token",
			"GET /ai_analysis/trigger?token=xxx - AI 分析触发端点（webhook 模式）",
			"GET /health - 健康检查",
		},
	}
	writeJSON(w, 200, resp)
}

// ============================================================================
// /health - 健康检查
// ============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	resp := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"service":   "nightingale-mute-callback",
		"ai": map[string]interface{}{
			"strategy": cfg.AIProviderStrategy,
			"openclaw": func() string { if cfg.OpenClawEnabled { return "enabled" }; return "disabled" }(),
			"iflyelf":  func() string { if cfg.IflyelfEnabled { return "enabled" }; return "disabled" }(),
		},
	}
	writeJSON(w, 200, resp)
}

// ============================================================================
// POST /mute/register - 注册屏蔽参数
// ============================================================================

func (s *Server) handleMuteRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, 400, map[string]string{"err": "JSON 解析失败"})
		return
	}

	if data["duration"] == nil || data["group_id"] == nil {
		writeJSON(w, 400, map[string]string{"err": "缺少必要参数"})
		return
	}

	token := muteTokenStore.GenToken(fmt.Sprintf("%v", data["duration"]), data)
	logger.Infof("注册屏蔽参数: token=%s, duration=%v", token, data["duration"])
	writeJSON(w, 200, map[string]string{"token": token})
}

// ============================================================================
// POST /ai_analysis/register - 注册 AI 分析参数
// ============================================================================

func (s *Server) handleAIRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, 400, map[string]string{"err": "JSON 解析失败"})
		return
	}

	if data["rule_name"] == nil {
		writeJSON(w, 400, map[string]string{"err": "缺少 rule_name"})
		return
	}

	token := aiTokenStore.GenToken("ai", data)
	logger.Infof("注册 AI 分析参数: token=%s, rule=%v", token, data["rule_name"])
	writeJSON(w, 200, map[string]string{"token": token})
}

// ============================================================================
// POST /group_chat/register - 注册拉群参数
// ============================================================================

func (s *Server) handleGroupChatRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, 400, map[string]string{"err": "JSON 解析失败"})
		return
	}

	if data["emails"] == nil || data["rule_name"] == nil {
		writeJSON(w, 400, map[string]string{"err": "缺少必要参数 (emails/rule_name)"})
		return
	}

	token := groupChatTokenStore.GenToken("gc", data)
	logger.Infof("注册拉群参数: token=%s, rule=%v", token, data["rule_name"])
	writeJSON(w, 200, map[string]string{"token": token})
}

// ============================================================================
// GET /ai_analysis/trigger - AI 分析触发（webhook 模式）
// ============================================================================

func (s *Server) handleAITrigger(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		w.WriteHeader(400)
		fmt.Fprint(w, renderAIToast("error", "❌ 缺少 token 参数"))
		return
	}

	aiData := aiTokenStore.Load(token)
	if aiData == nil {
		w.WriteHeader(404)
		fmt.Fprint(w, renderAIToast("error", "❌ 分析参数已过期，请重新触发告警"))
		return
	}

	if s.aiDedup.isDuplicate(token + "_webhook") {
		fmt.Fprint(w, renderAIToast("info", "🤖 分析已在进行中，请稍候查收卡片..."))
		return
	}

	// 异步执行 AI 分析
	go func() {
		result := s.ai.Analyze(aiData)
		card := buildAIResultCard(aiData, result)
		webhookTokens := getStringSlice(aiData, "_webhook_tokens")
		for _, wt := range webhookTokens {
			s.sendFeishuWebhookCard(wt, card)
		}
	}()

	fmt.Fprint(w, renderAIToast("success", "🤖 AI 正在分析，请稍候查收卡片..."))
}

// ============================================================================
// POST /feishu_callback - 飞书卡片交互回调
// ============================================================================

func (s *Server) handleFeishuCallback(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var data map[string]interface{}
	json.Unmarshal(body, &data)

	preview := string(body)
	if len(preview) > 500 {
		preview = preview[:500]
	}
	logger.Infof("收到飞书回调: %s", preview)

	// 1. URL 校验
	if data["type"] == "url_verification" {
		challenge, _ := data["challenge"].(string)
		writeJSON(w, 200, map[string]string{"challenge": challenge})
		return
	}

	// 2. 去重
	eventID := getNestedString(data, "header", "event_id")
	eventData := getMap(data, "event")
	openMsgID := getNestedString(eventData, "context", "open_message_id")
	if openMsgID == "" {
		openMsgID, _ = data["open_message_id"].(string)
	}
	action := getMap(data, "action")
	if len(action) == 0 {
		action = getMap(eventData, "action")
	}
	optionForDedup, _ := action["option"].(string)
	dedupKey := eventID
	if dedupKey == "" {
		dedupKey = openMsgID + "_" + optionForDedup
	}
	if dedupKey != "" && s.dedup.isDuplicate(dedupKey) {
		logger.Warningf("重复请求已忽略: key=%s", dedupKey)
		writeJSON(w, 200, map[string]interface{}{"toast": map[string]string{"type": "info", "content": "已处理"}})
		return
	}

	// 3. 分支处理
	actionValue := getMap(action, "value")
	actionType, _ := actionValue["action"].(string)

	// 3.1 AI 分析
	if actionType == "ai_analysis" {
		s.handleAIAnalysisCallback(w, actionValue, eventData, data)
		return
	}

	// 3.2 协同群
	if actionType == "group_chat" {
		s.handleGroupChatCallback(w, action, actionValue, eventData, data)
		return
	}

	// 3.3 屏蔽规则
	s.handleMuteCallback(w, action)
}

func (s *Server) handleAIAnalysisCallback(w http.ResponseWriter, actionValue map[string]interface{}, eventData, data map[string]interface{}) {
	token, _ := actionValue["token"].(string)
	if token == "" {
		writeJSON(w, 200, toastError("缺少 AI 分析参数"))
		return
	}

	aiData := aiTokenStore.Load(token)
	if aiData == nil {
		writeJSON(w, 200, toastError("分析参数已过期"))
		return
	}

	operator := getMap(eventData, "operator")
	if len(operator) == 0 {
		operator = getMap(data, "operator")
	}
	receiveID := getString(operator, "open_id")
	if receiveID == "" {
		receiveID = getString(operator, "user_id")
	}
	if receiveID == "" {
		receiveID, _ = data["open_id"].(string)
	}
	if receiveID == "" {
		writeJSON(w, 200, toastError("无法识别用户"))
		return
	}

	if s.aiDedup.isDuplicate(token + "_" + receiveID) {
		writeJSON(w, 200, toastInfo("🤖 分析已在进行中"))
		return
	}

	appID := getString(aiData, "_feishu_app_id")
	appSecret := getString(aiData, "_feishu_app_secret")
	if appID == "" || appSecret == "" {
		writeJSON(w, 200, toastError("飞书凭证缺失"))
		return
	}

	// 异步分析
	go func() {
		result := s.ai.Analyze(aiData)
		card := buildAIResultCard(aiData, result)
		fsToken, err := s.feishu.GetAccessToken(appID, appSecret)
		if err == nil {
			s.feishu.SendCard(fsToken, receiveID, "open_id", card)
		}
	}()

	writeJSON(w, 200, toastInfo("🤖 AI 正在分析，请稍候查收卡片..."))
}

func (s *Server) handleGroupChatCallback(w http.ResponseWriter, action map[string]interface{}, actionValue map[string]interface{}, eventData, data map[string]interface{}) {
	rawOption, _ := action["option"].(string)
	rawOption = strings.TrimSpace(rawOption)

	var gcAction, gcToken string
	if strings.HasPrefix(rawOption, "create_") {
		gcAction = "create"
		gcToken = rawOption[7:]
	} else if strings.HasPrefix(rawOption, "dismiss_") {
		gcAction = "dismiss"
		gcToken = rawOption[8:]
	} else {
		writeJSON(w, 200, toastError("未知的协同群操作"))
		return
	}

	gcData := groupChatTokenStore.Load(gcToken)
	if gcData == nil {
		writeJSON(w, 200, toastError("协同群参数已过期"))
		return
	}

	operator := getMap(eventData, "operator")
	if len(operator) == 0 {
		operator = getMap(data, "operator")
	}
	operatorOpenID := getString(operator, "open_id")
	if operatorOpenID == "" {
		operatorOpenID = getString(operator, "user_id")
	}
	if operatorOpenID == "" {
		writeJSON(w, 200, toastError("无法识别用户"))
		return
	}

	if s.gcDedup.isDuplicate(gcToken + "_" + gcAction + "_" + operatorOpenID) {
		writeJSON(w, 200, toastInfo("操作处理中，请稍候..."))
		return
	}

	appID := getString(gcData, "_feishu_app_id")
	appSecret := getString(gcData, "_feishu_app_secret")
	if appID == "" || appSecret == "" {
		writeJSON(w, 200, toastError("飞书凭证缺失"))
		return
	}

	if gcAction == "create" {
		existingChatID := getString(gcData, "chat_id")
		if existingChatID != "" {
			writeJSON(w, 200, toastInfo("协同群已存在，无需重复创建"))
			return
		}

		go s.asyncCreateChat(gcToken, gcData, operatorOpenID, appID, appSecret)
		writeJSON(w, 200, toastInfo("👥 正在创建协同群，请稍候..."))
		return
	}

	// dismiss
	chatID := getString(gcData, "chat_id")
	if chatID == "" {
		writeJSON(w, 200, toastWarn("尚未创建协同群"))
		return
	}

	go s.asyncDismissChat(gcToken, chatID, operatorOpenID, appID, appSecret)
	writeJSON(w, 200, toastInfo("🗑 正在解散协同群，请稍候..."))
}

// asyncCreateChat 异步创建协同群：建群、加管理员、发告警卡片、回写 token、通知点击者。
func (s *Server) asyncCreateChat(gcToken string, gcData map[string]interface{}, operatorOpenID, appID, appSecret string) {
	feishuToken, err := s.feishu.GetAccessToken(appID, appSecret)
	if err != nil {
		logger.Errorf("获取飞书 token 失败: %v", err)
		return
	}

	ruleName := getString(gcData, "rule_name")
	if ruleName == "" {
		ruleName = "未知告警"
	}
	groupName := getString(gcData, "group_name")
	if groupName == "" {
		groupName = "信息化监控告警"
	}
	chatName := fmt.Sprintf("[%s] %s", groupName, ruleName)
	emails := getStringSlice(gcData, "emails")

	chatID, ok, msg := s.feishu.CreateChat(feishuToken, chatName, operatorOpenID)
	if !ok || chatID == "" {
		logger.Errorf("群聊创建失败: %s", msg)
		failCard := buildSimpleNotifyCard("❌ 拉群失败", fmt.Sprintf("**失败原因:** %s\n\n请检查飞书权限或联系管理员。", msg), "red")
		s.feishu.SendCard(feishuToken, operatorOpenID, "open_id", failCard)
		return
	}
	logger.Infof("群聊创建成功: chat_id=%s", chatID)

	// 通过邮箱添加成员
	invalidEmails := s.feishu.AddMembersByEmail(feishuToken, chatID, emails)

	// 添加默认管理员（机器人保持群主，保证解散权限）
	adminEmail := s.cfg.DefaultAdminEmail
	if adminEmail != "" {
		s.feishu.AddChatManagers(feishuToken, chatID, []string{adminEmail})
	}

	// 发送告警卡片到新群
	snapshot := getMap(gcData, "_alert_card_snapshot")
	if len(snapshot) > 0 {
		alertCard := buildAlertCardForGroup(snapshot)
		s.feishu.SendCardToChat(feishuToken, chatID, alertCard)
	}

	// 回写 token
	groupChatTokenStore.Update(gcToken, map[string]interface{}{
		"chat_id":         chatID,
		"creator_open_id": operatorOpenID,
		"created_at":      time.Now().Unix(),
	})

	// 计算成功加入的成员
	invalidSet := make(map[string]bool)
	for _, e := range invalidEmails {
		invalidSet[e] = true
	}
	if adminEmail != "" {
		delete(invalidSet, adminEmail)
	}
	var membersLines []string
	membersLines = append(membersLines, "- 群主（您）")
	successCount := 1
	for _, em := range emails {
		if !invalidSet[em] {
			membersLines = append(membersLines, "- "+em)
			successCount++
		}
	}
	membersText := strings.Join(membersLines, "\n")
	content := fmt.Sprintf("**群名称:** %s\n\n**成员（%d 人）:**\n%s\n\n告警详情已发送到群中，请前往群聊查看。",
		chatName, successCount, membersText)
	successCard := buildSimpleNotifyCard("✅ 拉群成功", content, "green")
	s.feishu.SendCard(feishuToken, operatorOpenID, "open_id", successCard)
}

// asyncDismissChat 异步解散协同群：解散、回写清空 chat_id、通知点击者。
func (s *Server) asyncDismissChat(gcToken, chatID, operatorOpenID, appID, appSecret string) {
	feishuToken, err := s.feishu.GetAccessToken(appID, appSecret)
	if err != nil {
		logger.Errorf("获取飞书 token 失败: %v", err)
		return
	}

	ok, msg := s.feishu.DismissChat(feishuToken, chatID)
	if !ok {
		logger.Errorf("群聊解散失败: %s", msg)
		groupChatTokenStore.Update(gcToken, map[string]interface{}{
			"chat_id":      "",
			"dismissed_at": time.Now().Unix(),
			"dismissed_by": operatorOpenID,
		})
		failCard := buildSimpleNotifyCard("❌ 解散失败", fmt.Sprintf("**失败原因:** %s\n\n请在飞书中手动解散该群。", msg), "red")
		s.feishu.SendCard(feishuToken, operatorOpenID, "open_id", failCard)
		return
	}
	logger.Infof("群聊解散成功: chat_id=%s", chatID)

	groupChatTokenStore.Update(gcToken, map[string]interface{}{
		"chat_id":      "",
		"dismissed_at": time.Now().Unix(),
		"dismissed_by": operatorOpenID,
	})
	successCard := buildSimpleNotifyCard("✅ 解散成功", "协同群已解散。", "green")
	s.feishu.SendCard(feishuToken, operatorOpenID, "open_id", successCard)
}

func (s *Server) handleMuteCallback(w http.ResponseWriter, action map[string]interface{}) {
	optionEncoded, _ := action["option"].(string)
	if optionEncoded == "" {
		optionEncoded, _ = action["selected_value"].(string)
	}
	if optionEncoded == "" {
		writeJSON(w, 200, toastError("缺少 option 参数"))
		return
	}

	muteData := muteTokenStore.Load(optionEncoded)
	if muteData == nil {
		// 尝试 base64 解码兼容旧版
		decoded, err := base64.StdEncoding.DecodeString(optionEncoded)
		if err != nil {
			writeJSON(w, 200, toastError("option 解码失败"))
			return
		}
		json.Unmarshal(decoded, &muteData)
		if muteData == nil {
			writeJSON(w, 200, toastError("option 解析失败"))
			return
		}
	}

	duration := getString(muteData, "duration")
	groupID := int(getFloat(muteData, "group_id"))
	groupName := getString(muteData, "group_name")
	ruleName := getString(muteData, "rule_name")
	instance := getString(muteData, "instance")
	triggerTime := int64(getFloat(muteData, "trigger_time"))
	tagsArray := getStringSlice(muteData, "tags")
	isAggregated := getBool(muteData, "is_aggregated")
	var instances []string
	if isAggregated {
		instances = getStringSlice(muteData, "instances")
	}

	if duration == "" || groupID == 0 || triggerTime == 0 || len(tagsArray) == 0 {
		writeJSON(w, 200, toastError("参数不完整"))
		return
	}

	tags := convertTagsToAPIFormat(tagsArray)
	durationText := getDurationText(duration)

	// 取消屏蔽：查找并删除该告警的所有屏蔽规则
	if duration == "unmute" {
		matched, deleted, failed := s.findAndDeleteMuteRules(groupID, ruleName, instance, isAggregated, instances)
		if matched == 0 {
			writeJSON(w, 200, toastInfo("未找到该告警的屏蔽规则"))
		} else if failed == 0 {
			writeJSON(w, 200, toastSuccess(fmt.Sprintf("✓ 已取消 %d 条屏蔽规则", deleted)))
		} else {
			writeJSON(w, 200, toastWarn(fmt.Sprintf("⚠ 删除部分成功: %d/%d", deleted, matched)))
		}
		return
	}

	// 创建屏蔽规则
	muteTimeType, etime := calculateMuteTime(triggerTime, duration)
	note := fmt.Sprintf("%s-%s-%s-%d", groupName, ruleName, instance, triggerTime)

	success, msg := s.n9e.CreateMuteRule(groupID, note, tags, muteTimeType, triggerTime, etime)
	if success {
		writeJSON(w, 200, toastSuccess(fmt.Sprintf("✓ 告警屏蔽成功 (%s)", durationText)))
	} else {
		writeJSON(w, 200, toastError(fmt.Sprintf("✗ 屏蔽失败: %s", msg)))
	}
}

// findAndDeleteMuteRules 根据 rule_name + instance 匹配业务组下所有相关屏蔽规则并删除。
// 匹配逻辑：屏蔽规则 note 含 rule_name 且 tags 中 instance=xxx 命中目标实例集合。
// 返回 (matched, deleted, failed)。
func (s *Server) findAndDeleteMuteRules(groupID int, ruleName, instance string, isAggregated bool, instances []string) (int, int, int) {
	// 计算目标 instance 集合
	targetInstances := make(map[string]bool)
	if isAggregated {
		for _, ins := range instances {
			targetInstances[ins] = true
		}
	}
	if instance != "" && instance != "N/A" {
		targetInstances[instance] = true
	}
	if len(targetInstances) == 0 {
		logger.Warningf("未提供 instance，无法定位屏蔽规则: rule=%s", ruleName)
		return 0, 0, 0
	}

	allRules, err := s.n9e.ListMuteRules(groupID)
	if err != nil {
		logger.Errorf("查询屏蔽规则列表失败: group_id=%d, error=%v", groupID, err)
		return 0, 0, 0
	}
	logger.Infof("开始匹配屏蔽规则: group_id=%d, total=%d, rule=%s", groupID, len(allRules), ruleName)

	// 筛选匹配规则
	var matchedRules []map[string]interface{}
	for _, r := range allRules {
		note := getString(r, "note")
		if !strings.Contains(note, ruleName) {
			continue
		}
		ruleInstance := ""
		if tagsRaw, ok := r["tags"].([]interface{}); ok {
			for _, t := range tagsRaw {
				tag, ok := t.(map[string]interface{})
				if !ok {
					continue
				}
				if getString(tag, "key") == "instance" && getString(tag, "func") == "==" {
					ruleInstance = getString(tag, "value")
					break
				}
			}
		}
		if targetInstances[ruleInstance] {
			matchedRules = append(matchedRules, r)
		}
	}

	matched := len(matchedRules)
	logger.Infof("匹配到待删除规则: count=%d", matched)
	if matched == 0 {
		return 0, 0, 0
	}

	deleted, failed := 0, 0
	for _, r := range matchedRules {
		ruleID := int(getFloat(r, "id"))
		ok, msg := s.n9e.DeleteMuteRule(groupID, ruleID)
		if ok {
			deleted++
			logger.Infof("删除屏蔽规则: id=%d, note=%s", ruleID, getString(r, "note"))
		} else {
			failed++
			logger.Warningf("删除屏蔽规则失败: id=%d, msg=%s", ruleID, msg)
		}
	}
	logger.Infof("删除完成: deleted=%d, matched=%d", deleted, matched)
	return matched, deleted, failed
}

func (s *Server) sendFeishuWebhookCard(webhookToken string, card map[string]interface{}) {
	if webhookToken == "" {
		return
	}
	url := fmt.Sprintf("https://%s/open-apis/bot/v2/hook/%s", s.feishu.domain, webhookToken)
	payload := map[string]interface{}{"msg_type": "interactive", "card": card}
	body, _ := json.Marshal(payload)
	resp, err := s.feishu.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Errorf("webhook 推送失败: %v", err)
		return
	}
	defer resp.Body.Close()
	suffix := webhookToken
	if len(suffix) > 6 {
		suffix = suffix[len(suffix)-6:]
	}
	logger.Infof("webhook 推送成功: token_suffix=%s", suffix)
}

// ============================================================================
// 辅助函数
// ============================================================================

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func toastSuccess(msg string) map[string]interface{} {
	return map[string]interface{}{"toast": map[string]string{"type": "success", "content": msg}}
}

func toastInfo(msg string) map[string]interface{} {
	return map[string]interface{}{"toast": map[string]string{"type": "info", "content": msg}}
}

func toastWarn(msg string) map[string]interface{} {
	return map[string]interface{}{"toast": map[string]string{"type": "warning", "content": msg}}
}

func toastError(msg string) map[string]interface{} {
	return map[string]interface{}{"toast": map[string]string{"type": "error", "content": msg}}
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return make(map[string]interface{})
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getStringSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key].([]interface{}); ok {
		var result []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func getNestedString(m map[string]interface{}, keys ...string) string {
	current := m
	for i, k := range keys {
		if i == len(keys)-1 {
			return getString(current, k)
		}
		current = getMap(current, k)
		if len(current) == 0 {
			return ""
		}
	}
	return ""
}

func convertTagsToAPIFormat(tags []string) []map[string]interface{} {
	var result []map[string]interface{}
	for _, tag := range tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) == 2 {
			result = append(result, map[string]interface{}{
				"func":  "==",
				"key":   parts[0],
				"value": parts[1],
			})
		}
	}
	return result
}

func calculateMuteTime(triggerTime int64, duration string) (int, int64) {
	durationMap := map[string]int64{
		"1d": 86400,
		"3d": 259200,
		"6d": 518400,
		"1w": 604800,
		"3w": 1814400,
		"1m": 2592000,
	}
	if duration == "forever" {
		return 1, triggerTime + 86400*365*10
	}
	seconds := durationMap[duration]
	if seconds == 0 {
		seconds = 86400
	}
	return 0, triggerTime + seconds
}

func getDurationText(duration string) string {
	textMap := map[string]string{
		"1d": "1天", "3d": "3天", "6d": "6天",
		"1w": "1周", "3w": "3周", "1m": "1个月",
		"forever": "永久", "unmute": "取消屏蔽",
	}
	if t, ok := textMap[duration]; ok {
		return t
	}
	return duration
}
