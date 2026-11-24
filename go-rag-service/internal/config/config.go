package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Ollama   OllamaConfig   `mapstructure:"ollama"`
	API      APIConfig      `mapstructure:"api"`
	Strategy StrategyConfig `mapstructure:"strategy"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

type OllamaConfig struct {
	Host       string `mapstructure:"host"`
	LLMModel   string `mapstructure:"llm_model"`
	EmbedModel string `mapstructure:"embed_model"`
	Timeout    string `mapstructure:"timeout"`
}

func (o OllamaConfig) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(o.Timeout)
	if err != nil {
		return 120 * time.Second
	}
	return d
}

type APIConfig struct {
	BaseURL    string `mapstructure:"base_url"`
	APIKey     string `mapstructure:"api_key"`
	LLMModel   string `mapstructure:"llm_model"`
	EmbedModel string `mapstructure:"embed_model"`
	Timeout    string `mapstructure:"timeout"`
}

func (a *APIConfig) ResolveAPIKey() string {
	key := a.APIKey
	if strings.HasPrefix(key, "${") && strings.HasSuffix(key, "}") {
		envVar := key[2 : len(key)-1]
		return os.Getenv(envVar)
	}
	return key
}

func (a APIConfig) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(a.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

type StrategyConfig struct {
	Embed    string `mapstructure:"embed"`
	Generate string `mapstructure:"generate"`
}

func Load() *Config {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("../../configs/")
	viper.AddConfigPath("./configs/")

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("read config: %w", err))
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(fmt.Errorf("parse config: %w", err))
	}

	return &cfg
}
