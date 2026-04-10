package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AppConfig 应用的顶层配置结构体，与 config.yaml 中的字段一一映射
type AppConfig struct {
	Server  ServerConfig  `yaml:"server"`
	MySQL   MySQLConfig   `yaml:"mysql"`
	Kafka   KafkaConfig   `yaml:"kafka"`
	LLM     LLMConfig     `yaml:"llm"`
	Video   VideoConfig   `yaml:"video"`
	Storage StorageConfig `yaml:"storage"`
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
	Brokers []string `yaml:"brokers"` // Kafka Broker 地址列表
	Topic   string   `yaml:"topic"`   // 投递/消费的 Topic 名称
	GroupID string   `yaml:"group_id"` // 消费者组 ID
}

// LLMConfig 大模型 API 配置
type LLMConfig struct {
	Provider string `yaml:"provider"` // 供应商标识: "openai" 或 "doubao"（留空默认 openai）
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	BaseURL  string `yaml:"base_url"`
}

// VideoConfig 视频生成服务商的配置
type VideoConfig struct {
	Provider string `yaml:"provider"` // 当前使用的服务商标识（如 bytedance, kling）
	APIKey   string `yaml:"api_key"`
	BaseURL  string `yaml:"base_url"`
	Model    string `yaml:"model"`    // 大模型 ID，如 doubao-seedance-2-0-260128
}

// StorageConfig 本地文件存储路径配置
type StorageConfig struct {
	UploadDir string `yaml:"upload_dir"` // 用户上传的参考图片存放目录
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
