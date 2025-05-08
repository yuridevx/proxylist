package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"reflect"
)

type Config struct {
	DSN                string   `yaml:"dsn"`
	ZapProduction      bool     `yaml:"zap_production"`
	ZapLogLevel        string   `yaml:"zap_log_level"`
	ParallelTests      int      `yaml:"parallel_tests"`
	FetchItemUrl       string   `yaml:"fetch_item_url"`
	ProxyTimeoutS      int      `yaml:"proxy_timeout_s"`
	HostPortSourceList []string `yaml:"host_port_source_list"`
	UrlSourceList      []string `yaml:"url_source_list"`
	UseDBProxy         bool     `yaml:"use_db_proxy"`
}

func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func MergeStructs(target interface{}, source interface{}) {
	targetVal := reflect.ValueOf(target).Elem()
	sourceVal := reflect.ValueOf(source).Elem()
	for i := 0; i < sourceVal.NumField(); i++ {
		field := sourceVal.Field(i)
		if !field.IsZero() {
			targetVal.Field(i).Set(field)
		}
	}
}

func NewConfig() *Config {
	var paths []string

	homeDir, err := os.UserHomeDir()
	if err == nil {
		paths = append(paths, filepath.Join(homeDir, "proxy_list.yaml"))
	}

	pwd, err := os.Getwd()
	if err == nil {
		paths = append(paths, filepath.Join(pwd, "proxy_list.yaml"))
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		paths = append(paths, filepath.Join(exeDir, "proxy_list.yaml"))
	}

	finalConfig := &Config{
		ParallelTests: 15,
		ProxyTimeoutS: 30,
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			config, err := LoadConfigFromFile(path)
			if err == nil {
				MergeStructs(finalConfig, config)
			}
		}
	}

	finalConfig.ParallelTests = max(min(finalConfig.ParallelTests, 1000), 1)

	return finalConfig
}
