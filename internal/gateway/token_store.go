package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// Token 存储：与 Python 原版逻辑完全一致，持久化到文件以支持服务重启
// 多副本场景需挂载共享 RWX 卷（如 NFS/CephFS）到 DataDir
// ============================================================================

// TokenStore 接口定义（支持 file 和 redis 两种实现）
type TokenStore interface {
	GenToken(prefix string, data map[string]interface{}) string
	Load(token string) map[string]interface{}
	Update(token string, patch map[string]interface{}) bool
	GC()
}

// ============================================================================
// FileTokenStore - 文件存储实现
// ============================================================================

type FileTokenStore struct {
	dir string
	ttl int64
	mu  sync.RWMutex
}

// NewFileTokenStore 创建文件 token 存储实例，dir 为持久化目录，ttl 单位秒。
func NewFileTokenStore(dir string, ttl int64) (*FileTokenStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建 token 目录失败: %w", err)
	}
	return &FileTokenStore{dir: dir, ttl: ttl}, nil
}

// GenToken 生成短 token 并持久化数据到文件。
func (ts *FileTokenStore) GenToken(prefix string, data map[string]interface{}) string {
	buf := make([]byte, 6)
	rand.Read(buf)
	token := prefix + "_" + hex.EncodeToString(buf)

	ts.mu.Lock()
	defer ts.mu.Unlock()

	payload := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"data":      data,
	}
	fp := filepath.Join(ts.dir, token+".json")
	f, err := os.Create(fp)
	if err != nil {
		logger.Errorf("token 文件创建失败: token=%s, error=%v", token, err)
		return token
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(payload)

	return token
}

// Load 读取 token 对应数据；过期或不存在返回 nil。
func (ts *FileTokenStore) Load(token string) map[string]interface{} {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	fp := filepath.Join(ts.dir, token+".json")
	data, err := os.ReadFile(fp)
	if err != nil {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		logger.Warningf("token 文件 JSON 解析失败: token=%s, error=%v", token, err)
		return nil
	}

	tsFloat, ok := payload["timestamp"].(float64)
	if !ok {
		return nil
	}
	ts64 := int64(tsFloat)
	if time.Now().Unix()-ts64 > ts.ttl {
		// 过期，删除文件
		os.Remove(fp)
		return nil
	}

	dataMap, ok := payload["data"].(map[string]interface{})
	if !ok {
		return nil
	}
	return dataMap
}

// Update 更新 token 文件中的字段（用于回写 chat_id 等）。
func (ts *FileTokenStore) Update(token string, patch map[string]interface{}) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	fp := filepath.Join(ts.dir, token+".json")
	data, err := os.ReadFile(fp)
	if err != nil {
		return false
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}

	dataMap, ok := payload["data"].(map[string]interface{})
	if !ok {
		return false
	}
	for k, v := range patch {
		dataMap[k] = v
	}

	f, err := os.Create(fp)
	if err != nil {
		return false
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(payload)
	return true
}

// GC 清理过期 token 文件。
func (ts *FileTokenStore) GC() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	entries, err := os.ReadDir(ts.dir)
	if err != nil {
		return
	}

	now := time.Now().Unix()
	cleaned := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		fp := filepath.Join(ts.dir, e.Name())
		data, err := os.ReadFile(fp)
		if err != nil {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(data, &payload); err != nil {
			continue
		}
		tsFloat, ok := payload["timestamp"].(float64)
		if !ok {
			continue
		}
		if now-int64(tsFloat) > ts.ttl {
			os.Remove(fp)
			cleaned++
		}
	}
	if cleaned > 0 {
		logger.Infof("清理过期 token: dir=%s, count=%d", ts.dir, cleaned)
	}
}

// ============================================================================
// RedisTokenStore - Redis 存储实现（TODO: 待实现）
// ============================================================================

// RedisTokenStore Redis 存储实现（暂未实现，需要 redis.Client 注入）
type RedisTokenStore struct {
	// client *redis.Client
	prefix string
	ttl    int64
}

// NewRedisTokenStore 创建 Redis token 存储实例
func NewRedisTokenStore(prefix string, ttl int64) (*RedisTokenStore, error) {
	// TODO: 需要从外部注入 redis.Client
	logger.Warning("RedisTokenStore 暂未实现，降级到 FileTokenStore")
	return nil, fmt.Errorf("RedisTokenStore 未实现")
}

func (r *RedisTokenStore) GenToken(prefix string, data map[string]interface{}) string {
	// TODO: 实现 Redis SET
	return ""
}

func (r *RedisTokenStore) Load(token string) map[string]interface{} {
	// TODO: 实现 Redis GET
	return nil
}

func (r *RedisTokenStore) Update(token string, patch map[string]interface{}) bool {
	// TODO: 实现 Redis UPDATE
	return false
}

func (r *RedisTokenStore) GC() {
	// Redis 自动过期，无需手动 GC
}

// ============================================================================
// 全局 token 存储
// ============================================================================

var (
	muteTokenStore      TokenStore
	aiTokenStore        TokenStore
	groupChatTokenStore TokenStore
)

func InitTokenStores(cfg *Config) error {
	var err error

	if cfg.TokenStoreType == "redis" {
		// TODO: 当 Redis 可用时，创建 RedisTokenStore
		logger.Warning("TokenStoreType=redis 暂不支持，降级到 file")
	}

	// 使用 FileTokenStore
	muteStore, err := NewFileTokenStore(filepath.Join(cfg.DataDir, "tokens", "mute"), cfg.MuteTokenTTL)
	if err != nil {
		return fmt.Errorf("初始化 muteTokenStore 失败: %w", err)
	}
	muteTokenStore = muteStore

	aiStore, err := NewFileTokenStore(filepath.Join(cfg.DataDir, "tokens", "ai"), cfg.AITokenTTL)
	if err != nil {
		return fmt.Errorf("初始化 aiTokenStore 失败: %w", err)
	}
	aiTokenStore = aiStore

	gcStore, err := NewFileTokenStore(filepath.Join(cfg.DataDir, "tokens", "groupchat"), cfg.GroupChatTokenTTL)
	if err != nil {
		return fmt.Errorf("初始化 groupChatTokenStore 失败: %w", err)
	}
	groupChatTokenStore = gcStore

	return nil
}

// 定期 GC（后台 goroutine）
func StartTokenGC() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			muteTokenStore.GC()
			aiTokenStore.GC()
			groupChatTokenStore.GC()
		}
	}()
}
