package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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
// RedisTokenStore - Redis 存储实现（多副本共享，键自动过期）
// ============================================================================

// RedisTokenStore 使用 Redis 存储 token，天然支持多副本共享与自动过期。
// 键格式: {prefix}:{token}，value 为 data 的 JSON 序列化。
type RedisTokenStore struct {
	client redis.UniversalClient
	prefix string
	ttl    time.Duration
}

// NewRedisTokenStore 创建 Redis token 存储实例。
// client 为 redis.UniversalClient，兼容单机/哨兵/集群三种模式。
func NewRedisTokenStore(client redis.UniversalClient, prefix string, ttlSeconds int64) *RedisTokenStore {
	return &RedisTokenStore{
		client: client,
		prefix: prefix,
		ttl:    time.Duration(ttlSeconds) * time.Second,
	}
}

func (r *RedisTokenStore) key(token string) string {
	return r.prefix + ":" + token
}

// GenToken 生成 token 并写入 Redis（带 TTL 自动过期）。
func (r *RedisTokenStore) GenToken(prefix string, data map[string]interface{}) string {
	buf := make([]byte, 6)
	rand.Read(buf)
	token := prefix + "_" + hex.EncodeToString(buf)

	payload, err := json.Marshal(data)
	if err != nil {
		logger.Errorf("redis token 序列化失败: token=%s, error=%v", token, err)
		return token
	}

	ctx := context.Background()
	if err := r.client.Set(ctx, r.key(token), payload, r.ttl).Err(); err != nil {
		logger.Errorf("redis token 写入失败: token=%s, error=%v", token, err)
	}
	return token
}

// Load 从 Redis 读取 token 数据；不存在或过期返回 nil。
func (r *RedisTokenStore) Load(token string) map[string]interface{} {
	ctx := context.Background()
	val, err := r.client.Get(ctx, r.key(token)).Bytes()
	if err != nil {
		if err != redis.Nil {
			logger.Warningf("redis token 读取失败: token=%s, error=%v", token, err)
		}
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(val, &data); err != nil {
		logger.Warningf("redis token JSON 解析失败: token=%s, error=%v", token, err)
		return nil
	}
	return data
}

// Update 更新 token 中的字段（保留剩余 TTL）。
func (r *RedisTokenStore) Update(token string, patch map[string]interface{}) bool {
	ctx := context.Background()
	key := r.key(token)

	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return false
	}

	var data map[string]interface{}
	if err := json.Unmarshal(val, &data); err != nil {
		return false
	}
	for k, v := range patch {
		data[k] = v
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return false
	}

	// 保留剩余 TTL：KEEPTTL 需 Redis 6.0+，此处显式读取 TTL 后重设，兼容性更好
	ttl, err := r.client.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		ttl = r.ttl
	}
	if err := r.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		logger.Errorf("redis token 更新失败: token=%s, error=%v", token, err)
		return false
	}
	return true
}

// GC Redis 键自动过期，无需手动清理。
func (r *RedisTokenStore) GC() {}

// ============================================================================
// 全局 token 存储
// ============================================================================

var (
	muteTokenStore      TokenStore
	aiTokenStore        TokenStore
	groupChatTokenStore TokenStore
)

func InitTokenStores(cfg *Config) error {
	if cfg.TokenStoreType == "redis" {
		if cfg.RedisAddr == "" {
			logger.Warning("TOKEN_STORE_TYPE=redis 但 REDIS_ADDR 为空，降级到 file")
		} else {
			// 解析多地址（逗号分隔，用于集群/哨兵）
			addrs := strings.Split(cfg.RedisAddr, ",")
			for i := range addrs {
				addrs[i] = strings.TrimSpace(addrs[i])
			}

			// UniversalOptions 兼容单机/哨兵/集群三种模式
			opts := &redis.UniversalOptions{
				Addrs:    addrs,
				Username: cfg.RedisUsername,
				Password: cfg.RedisPassword,
				DB:       cfg.RedisDB,
			}

			// 根据显式 RedisMode 创建对应客户端，避免 NewUniversalClient
			// 在单种子地址集群场景下误判为单机。
			var client redis.UniversalClient
			switch strings.ToLower(cfg.RedisMode) {
			case "cluster":
				// 集群模式：即使只给单个种子地址也强制走 ClusterClient。DB 被忽略。
				client = redis.NewClusterClient(opts.Cluster())
				logger.Infof("Redis Cluster 模式: addrs=%v", addrs)
			case "sentinel":
				// 哨兵模式：Addrs 为哨兵节点，MasterName 必填
				opts.MasterName = cfg.RedisMasterName
				client = redis.NewFailoverClient(opts.Failover())
				logger.Infof("Redis Sentinel 模式: sentinels=%v, master=%s", addrs, cfg.RedisMasterName)
			default:
				// standalone：单机模式，仅用第一个地址
				if len(addrs) > 1 {
					logger.Warningf("REDIS_MODE=%s 但 REDIS_ADDR 有多个地址，仅使用第一个: %s", cfg.RedisMode, addrs[0])
				}
				client = redis.NewClient(opts.Simple())
				logger.Infof("Redis Standalone 模式: addr=%s", addrs[0])
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := client.Ping(ctx).Err(); err != nil {
				return fmt.Errorf("redis 连接失败(mode=%s, addrs=%v): %w", cfg.RedisMode, addrs, err)
			}

			muteTokenStore = NewRedisTokenStore(client, cfg.RedisPrefix+":mute", cfg.MuteTokenTTL)
			aiTokenStore = NewRedisTokenStore(client, cfg.RedisPrefix+":ai", cfg.AITokenTTL)
			groupChatTokenStore = NewRedisTokenStore(client, cfg.RedisPrefix+":groupchat", cfg.GroupChatTokenTTL)
			logger.Infof("RedisTokenStore 初始化成功: mode=%s, db=%d, prefix=%s", cfg.RedisMode, cfg.RedisDB, cfg.RedisPrefix)
			return nil
		}
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

	logger.Infof("使用 FileTokenStore: dir=%s", filepath.Join(cfg.DataDir, "tokens"))
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
