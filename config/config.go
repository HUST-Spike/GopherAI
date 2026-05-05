package config

import (
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/BurntSushi/toml"
)

type MainConfig struct {
	Port    int    `toml:"port"`
	AppName string `toml:"appName"`
	Host    string `toml:"host"`
}

type EmailConfig struct {
	Authcode string `toml:"authcode"`
	Email    string `toml:"email"`
}

type RedisConfig struct {
	RedisPort     int    `toml:"port"`
	RedisDb       int    `toml:"db"`
	RedisHost     string `toml:"host"`
	RedisPassword string `toml:"password"`
}

type MysqlConfig struct {
	MysqlPort         int    `toml:"port"`
	MysqlHost         string `toml:"host"`
	MysqlUser         string `toml:"user"`
	MysqlPassword     string `toml:"password"`
	MysqlDatabaseName string `toml:"databaseName"`
	MysqlCharset      string `toml:"charset"`
}

type JwtConfig struct {
	ExpireDuration int    `toml:"expire_duration"`
	Issuer         string `toml:"issuer"`
	Subject        string `toml:"subject"`
	Key            string `toml:"key"`
}

type Rabbitmq struct {
	RabbitmqPort     int    `toml:"port"`
	RabbitmqHost     string `toml:"host"`
	RabbitmqUsername string `toml:"username"`
	RabbitmqPassword string `toml:"password"`
	RabbitmqVhost    string `toml:"vhost"`
}

type RagModelConfig struct {
	RagEmbeddingModel string `toml:"embeddingModel"`
	RagChatModelName  string `toml:"chatModelName"`
	RagDocDir         string `toml:"docDir"`
	RagBaseUrl        string `toml:"baseUrl"`
	RagDimension      int    `toml:"dimension"`
}

type DocumentConfig struct {
	UploadDir     string   `toml:"uploadDir"`
	MaxFileSizeMB int      `toml:"maxFileSizeMB"`
	AllowedExts   []string `toml:"allowedExts"`
}

type DocumentIndexConfig struct {
	Exchange   string `toml:"exchange"`
	Queue      string `toml:"queue"`
	RoutingKey string `toml:"routingKey"`
}

type VoiceServiceConfig struct {
	VoiceServiceApiKey    string `toml:"voiceServiceApiKey"`
	VoiceServiceSecretKey string `toml:"voiceServiceSecretKey"`
}

type MCPConfig struct {
	Enabled    bool   `toml:"enabled"`
	ServerAddr string `toml:"serverAddr"`
	ServerPath string `toml:"serverPath"`
	AutoStart  bool   `toml:"autoStart"`
}

type AIModelConfig struct {
	DefaultModelType string `toml:"defaultModelType"`
}

type SkillConfig struct {
	EnabledSkills []string `toml:"enabledSkills"`
}

// SmartChatConfig governs the unified "all-in-one" chat (modelType=5).
// Each switch is a feature toggle that can be flipped at runtime via env
// vars (ENABLE_TOOLS / ENABLE_SKILLS / ENABLE_AGENT_LOOP) for testing in
// isolation. All values are intentionally optional so an empty toml block
// still yields a usable default (everything on, ten-second tool calls,
// three retries, ten reasoning rounds).
type SmartChatConfig struct {
	EnableTools        *bool `toml:"enableTools"`
	EnableSkills       *bool `toml:"enableSkills"`
	EnableAgentLoop    *bool `toml:"enableAgentLoop"`
	ToolCallTimeoutMs  int   `toml:"toolCallTimeoutMs"`
	ToolCallMaxRetries int   `toml:"toolCallMaxRetries"`
	ToolCallMaxRounds  int   `toml:"toolCallMaxRounds"`
}

type Config struct {
	MainConfig          `toml:"mainConfig"`
	EmailConfig         `toml:"emailConfig"`
	RedisConfig         `toml:"redisConfig"`
	MysqlConfig         `toml:"mysqlConfig"`
	JwtConfig           `toml:"jwtConfig"`
	Rabbitmq            `toml:"rabbitmqConfig"`
	RagModelConfig      `toml:"ragModelConfig"`
	DocumentConfig      `toml:"documentConfig"`
	DocumentIndexConfig `toml:"documentIndexConfig"`
	VoiceServiceConfig  `toml:"voiceServiceConfig"`
	MCPConfig           `toml:"mcpConfig"`
	AIModelConfig       `toml:"aiModelConfig"`
	SkillConfig         `toml:"skillConfig"`
	SmartChatConfig     `toml:"smartChatConfig"`
}

type RedisKeyConfig struct {
	CaptchaPrefix string
}

var DefaultRedisKeyConfig = RedisKeyConfig{
	CaptchaPrefix: "captcha:%s",
}

var (
	config     *Config
	configOnce sync.Once
)

func GetConfig() *Config {
	configOnce.Do(func() {
		config = new(Config)
		if _, err := toml.DecodeFile("config/config.toml", config); err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
	})
	return config
}

// MCPServerURL returns the full MCP server URL built from config.
func (c *Config) MCPServerURL() string {
	return "http://localhost" + c.MCPConfig.ServerAddr + c.MCPConfig.ServerPath
}

func (c *Config) DocumentUploadDir() string {
	if c.DocumentConfig.UploadDir == "" {
		return "uploads"
	}
	return c.DocumentConfig.UploadDir
}

func (c *Config) DocumentMaxFileSizeBytes() int64 {
	if c.DocumentConfig.MaxFileSizeMB <= 0 {
		return 20 * 1024 * 1024
	}
	return int64(c.DocumentConfig.MaxFileSizeMB) * 1024 * 1024
}

func (c *Config) DocumentIndexExchange() string {
	if c.DocumentIndexConfig.Exchange == "" {
		return "gopherai.document"
	}
	return c.DocumentIndexConfig.Exchange
}

func (c *Config) DocumentIndexQueue() string {
	if c.DocumentIndexConfig.Queue == "" {
		return "gopherai.document.index"
	}
	return c.DocumentIndexConfig.Queue
}

func (c *Config) DocumentIndexRoutingKey() string {
	if c.DocumentIndexConfig.RoutingKey == "" {
		return "document.uploaded"
	}
	return c.DocumentIndexConfig.RoutingKey
}

// SmartChat returns SmartChatConfig with env-var overrides applied. The
// pointer-typed switches let us tell apart "explicit false" from "left at
// default" in toml; env vars further override toml so demo-time toggles
// work without restarting nothing more than the process.
func (c *Config) SmartChat() SmartChatRuntime {
	enable := func(field *bool, envKey string, def bool) bool {
		if v, ok := envBool(envKey); ok {
			return v
		}
		if field != nil {
			return *field
		}
		return def
	}
	maxRounds := c.SmartChatConfig.ToolCallMaxRounds
	if v, ok := envInt("TOOL_CALL_MAX_ROUNDS"); ok {
		maxRounds = v
	}
	if maxRounds <= 0 {
		maxRounds = 10
	}
	return SmartChatRuntime{
		EnableTools:        enable(c.SmartChatConfig.EnableTools, "ENABLE_TOOLS", true),
		EnableSkills:       enable(c.SmartChatConfig.EnableSkills, "ENABLE_SKILLS", true),
		EnableAgentLoop:    enable(c.SmartChatConfig.EnableAgentLoop, "ENABLE_AGENT_LOOP", true),
		ToolCallTimeoutMs:  c.SmartChatConfig.ToolCallTimeoutMs,
		ToolCallMaxRetries: c.SmartChatConfig.ToolCallMaxRetries,
		ToolCallMaxRounds:  maxRounds,
	}
}

// SmartChatRuntime is the materialized config (no nil pointers, env vars
// already applied) consumed by SmartModel.
type SmartChatRuntime struct {
	EnableTools        bool
	EnableSkills       bool
	EnableAgentLoop    bool
	ToolCallTimeoutMs  int
	ToolCallMaxRetries int
	ToolCallMaxRounds  int
}

func envBool(key string) (bool, bool) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return false, false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false
	}
	return v, true
}

func envInt(key string) (int, bool) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}
