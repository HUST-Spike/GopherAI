package config

import (
	"log"
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

type Config struct {
	MainConfig         `toml:"mainConfig"`
	EmailConfig        `toml:"emailConfig"`
	RedisConfig        `toml:"redisConfig"`
	MysqlConfig        `toml:"mysqlConfig"`
	JwtConfig          `toml:"jwtConfig"`
	Rabbitmq           `toml:"rabbitmqConfig"`
	RagModelConfig     `toml:"ragModelConfig"`
	VoiceServiceConfig `toml:"voiceServiceConfig"`
	MCPConfig          `toml:"mcpConfig"`
	AIModelConfig      `toml:"aiModelConfig"`
	SkillConfig        `toml:"skillConfig"`
}

type RedisKeyConfig struct {
	CaptchaPrefix   string
	IndexName       string
	IndexNamePrefix string
}

var DefaultRedisKeyConfig = RedisKeyConfig{
	CaptchaPrefix:   "captcha:%s",
	IndexName:       "rag_docs:%s:idx",
	IndexNamePrefix: "rag_docs:%s:",
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
