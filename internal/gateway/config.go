package gateway

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ============================================================================
// 配置：所有可调项均从环境变量读取，保持与原 Python 版本完全一致的变量名与默认值
// ============================================================================

// Config 汇总服务运行所需的全部配置。
type Config struct {
	// 监听
	ListenHost string
	ListenPort int

	// N9E API
	N9EAPIURL   string
	N9EAPIToken string

	// 龙虾（OpenClaw）
	OpenClawEnabled  bool
	OpenClawAPIURL   string
	OpenClawAPIToken string
	OpenClawModel    string

	// iflyelf 专属AI
	IflyelfEnabled  bool
	IflyelfAPIURL   string
	IflyelfAPIToken string
	IflyelfModel    string

	// AI 调度
	AIProviderStrategy string
	AITimeout          int // 秒
	AIMaxTokens        int

	// 飞书
	FeishuDomain string

	// 重试
	RetryMaxAttempts   int
	RetryDelayBase     int
	RetryDelayMax      int
	RetryBackoffFactor int

	// API 超时
	APITimeout        int
	APIConnectTimeout int

	// 数据目录（token 持久化）；容器内建议挂载共享卷以支持多副本
	DataDir string

	// 日志级别
	LogLevel string

	// token TTL（秒）
	MuteTokenTTL      int64
	AITokenTTL        int64
	GroupChatTokenTTL int64

	// 默认管理员邮箱
	DefaultAdminEmail string

	// TokenStore 类型：file | redis
	TokenStoreType string

	// Redis 连接（TokenStoreType=redis 时使用）
	RedisAddr     string
	RedisUsername string
	RedisPassword string
	RedisDB       int
	RedisPrefix   string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return strings.EqualFold(strings.TrimSpace(v), "true")
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return def
}

func getenvInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
		return n
	}
	return def
}

// LoadConfig 从环境变量装配配置。
//
// 原 Python 服务里以字面量硬编码的 N9E_API_URL / N9E_API_TOKEN /
// FEISHU_DOMAIN / DEFAULT_ADMIN_EMAIL / LOG_DIR 等，这里统一提升为环境变量，
// 但默认值与原代码保持一致，做到「不配置也能原样运行」。
func LoadConfig() *Config {
	return &Config{
		ListenHost: getenv("LISTEN_HOST", "0.0.0.0"),
		ListenPort: getenvInt("LISTEN_PORT", 5000),

		N9EAPIURL:   getenv("N9E_API_URL", ""),
		N9EAPIToken: getenv("N9E_API_TOKEN", ""),

		OpenClawEnabled:  getenvBool("OPENCLAW_ENABLED", false),
		OpenClawAPIURL:   getenv("OPENCLAW_API_URL", ""),
		OpenClawAPIToken: getenv("OPENCLAW_API_TOKEN", ""),
		OpenClawModel:    getenv("OPENCLAW_MODEL", "openclaw/default"),

		IflyelfEnabled:  getenvBool("IFLYELF_ENABLED", false),
		IflyelfAPIURL:   getenv("IFLYELF_API_URL", ""),
		IflyelfAPIToken: getenv("IFLYELF_API_TOKEN", ""),
		IflyelfModel:    getenv("IFLYELF_MODEL", "auto"),

		AIProviderStrategy: getenv("AI_PROVIDER_STRATEGY", "openclaw_first"),
		AITimeout:          getenvInt("AI_TIMEOUT", 300),
		AIMaxTokens:        getenvInt("AI_MAX_TOKENS", 1500),

		FeishuDomain: getenv("FEISHU_DOMAIN", "open.feishu.cn"),

		RetryMaxAttempts:   getenvInt("RETRY_MAX_ATTEMPTS", 3),
		RetryDelayBase:     getenvInt("RETRY_DELAY_BASE", 2),
		RetryDelayMax:      getenvInt("RETRY_DELAY_MAX", 10),
		RetryBackoffFactor: getenvInt("RETRY_BACKOFF_FACTOR", 2),

		APITimeout:        getenvInt("API_TIMEOUT", 10),
		APIConnectTimeout: getenvInt("API_CONNECT_TIMEOUT", 5),

		// 容器内默认写到 /data/gateway（可挂共享卷）
		DataDir:  getenv("DATA_DIR", "/data/gateway"),
		LogLevel: getenv("LOG_LEVEL", "INFO"),

		MuteTokenTTL:      getenvInt64("MUTE_TOKEN_TTL", 30*86400),
		AITokenTTL:        getenvInt64("AI_TOKEN_TTL", 30*86400),
		GroupChatTokenTTL: getenvInt64("GROUP_CHAT_TOKEN_TTL", 30*86400),

		DefaultAdminEmail: getenv("DEFAULT_ADMIN_EMAIL", ""),

		TokenStoreType: getenv("TOKEN_STORE_TYPE", "file"),

		RedisAddr:     getenv("REDIS_ADDR", ""),
		RedisUsername: getenv("REDIS_USERNAME", ""),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       getenvInt("REDIS_DB", 0),
		RedisPrefix:   getenv("REDIS_PREFIX", "n9e_gateway"),
	}
}

// Validate 校验关键配置，缺失必填项时返回错误（避免使用空/明文默认凭据）。
func (c *Config) Validate() error {
	var missing []string
	if c.N9EAPIURL == "" {
		missing = append(missing, "N9E_API_URL")
	}
	if c.N9EAPIToken == "" {
		missing = append(missing, "N9E_API_TOKEN")
	}
	if c.OpenClawEnabled && (c.OpenClawAPIURL == "" || c.OpenClawAPIToken == "") {
		missing = append(missing, "OPENCLAW_API_URL/OPENCLAW_API_TOKEN(OPENCLAW_ENABLED=true 时必填)")
	}
	if c.IflyelfEnabled && (c.IflyelfAPIURL == "" || c.IflyelfAPIToken == "") {
		missing = append(missing, "IFLYELF_API_URL/IFLYELF_API_TOKEN(IFLYELF_ENABLED=true 时必填)")
	}
	if c.TokenStoreType == "redis" && c.RedisAddr == "" {
		missing = append(missing, "REDIS_ADDR(TOKEN_STORE_TYPE=redis 时必填)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("缺少必填环境变量: %s", strings.Join(missing, ", "))
	}
	return nil
}
