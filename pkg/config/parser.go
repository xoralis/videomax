package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AppConfig 应用的顶层配置结构体，与 config.yaml 中的字段一一映射
type AppConfig struct {
	Server    ServerConfig    `yaml:"server"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Kafka     KafkaConfig     `yaml:"kafka"`
	LLM       LLMConfig       `yaml:"llm"`
	Video     VideoConfig     `yaml:"video"`
	Storage   StorageConfig   `yaml:"storage"`
	Log       LogConfig       `yaml:"log"`
	LangSmith LangSmithConfig `yaml:"langsmith"`
	JWT       JWTConfig       `yaml:"jwt"`
}

// ServerConfig HTTP 服务相关配置
type ServerConfig struct {
	Port int `yaml:"port"`
}

// MySQLConfig 数据库连接配置
type MySQLConfig struct {
	DSN string `yaml:"dsn"`
}

// KafkaConfig 消息队列连接配置
type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`  // Kafka Broker 地址列表
	Topic   string   `yaml:"topic"`    // 投递/消费的 Topic 名称
	GroupID string   `yaml:"group_id"` // 消费者组 ID
}

// LLMConfig 大模型 API 配置
type LLMConfig struct {
	Provider string `yaml:"provider"` // 供应商标识: "openai" 或 "doubao"（留空默认 openai）
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	BaseURL  string `yaml:"base_url"`
}

// VideoProviderConfig 单个视频生成服务商的配置
type VideoProviderConfig struct {
	Name     string `yaml:"name"`     // 模型标识名，同时作为前端选择时的 key（如 doubao-seedance-1-0-pro-250528）
	Provider string `yaml:"provider"` // 服务商类型标识（bytedance / kling）
	APIKey   string `yaml:"api_key"`
	BaseURL  string `yaml:"base_url"` // 留空使用各服务商默认地址
}

// VideoConfig 视频生成服务商配置，支持多个 Provider 并行注册
type VideoConfig struct {
	Providers []VideoProviderConfig `yaml:"providers"`
}

// StorageConfig 本地文件存储路径配置
type StorageConfig struct {
	UploadDir string `yaml:"upload_dir"` // 用户上传的参考图片存放目录
}

// JWTConfig JWT 认证配置
type JWTConfig struct {
	Secret     string `yaml:"secret"`      // 签名密钥
	ExpireDays int    `yaml:"expire_days"` // Token 有效天数，默认 7
}

// LangSmithConfig LangSmith 链路追踪配置
type LangSmithConfig struct {
	Enabled     bool   `yaml:"enabled"`      // 是否启用 LangSmith 链路追踪
	APIKey      string `yaml:"api_key"`      // LangSmith API Key
	ProjectName string `yaml:"project_name"` // LangSmith 项目名称
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `yaml:"level"`        // 日志级别: debug, info, warn, error, fatal
	Mode       string `yaml:"mode"`         // 日志模式: console, file, both
	FilePath   string `yaml:"file_path"`    // 文件日志路径
	Format     string `yaml:"format"`       // 日志格式: console, json
	MaxSizeMB  int    `yaml:"max_size_mb"`  // 单个日志文件最大大小（MB）
	MaxBackups int    `yaml:"max_backups"`  // 最大保留日志文件数量
	MaxAgeDays int    `yaml:"max_age_days"` // 日志保留天数
	Compress   bool   `yaml:"compress"`     // 是否压缩日志
}

// Load 从指定 YAML 文件路径加载并解析配置
func Load(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}
