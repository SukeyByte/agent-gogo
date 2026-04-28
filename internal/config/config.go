package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sukeke/agent-gogo/internal/provider"
)

type Config struct {
	LLM           LLMConfig
	Browser       BrowserConfig
	Storage       StorageConfig
	Runtime       RuntimeConfig
	Communication CommunicationConfig
}

type LLMConfig struct {
	Provider        string
	Model           string
	BaseURL         string
	APIKey          string
	Timeout         time.Duration
	ThinkingEnabled bool
	ReasoningEffort string
}

type BrowserConfig struct {
	Provider         string
	MCPURL           string
	AutoStartMCP     bool
	DebugPort        int
	ChromePath       string
	UserDataDir      string
	Headless         bool
	MaxSummaryLength int
	Timeout          time.Duration
}

type StorageConfig struct {
	SQLitePath string
}

type RuntimeConfig struct {
	MaxTasksPerProject int
}

type CommunicationConfig struct {
	ChannelID string
	SessionID string
}

func Load(path string) (Config, error) {
	cfg := Default()
	configPath := firstNonEmpty(path, os.Getenv("AGENT_GOGO_CONFIG"), existingConfigPath())
	if configPath != "" {
		if err := applyYAMLFile(&cfg, configPath); err != nil {
			return Config{}, err
		}
	}
	applyEnv(&cfg)
	if cfg.LLM.Timeout <= 0 {
		cfg.LLM.Timeout = 120 * time.Second
	}
	if cfg.Browser.Timeout <= 0 {
		cfg.Browser.Timeout = 60 * time.Second
	}
	if cfg.Browser.MaxSummaryLength <= 0 {
		cfg.Browser.MaxSummaryLength = 12000
	}
	if cfg.Browser.MCPURL == "" {
		cfg.Browser.MCPURL = "http://127.0.0.1:9222"
	}
	if cfg.Browser.DebugPort == 0 {
		cfg.Browser.DebugPort = 9223
	}
	if cfg.Storage.SQLitePath == "" {
		cfg.Storage.SQLitePath = "./data/agent.db"
	}
	if cfg.Communication.ChannelID == "" {
		cfg.Communication.ChannelID = "cli"
	}
	if cfg.Communication.SessionID == "" {
		cfg.Communication.SessionID = "local"
	}
	return cfg, nil
}

func Default() Config {
	return Config{
		LLM: LLMConfig{
			Provider:        "deepseek",
			Model:           provider.DefaultDeepSeekModel,
			BaseURL:         provider.DefaultDeepSeekBaseURL,
			Timeout:         120 * time.Second,
			ThinkingEnabled: false,
		},
		Browser: BrowserConfig{
			Provider:         "chrome_mcp",
			MCPURL:           "http://127.0.0.1:9222",
			AutoStartMCP:     true,
			DebugPort:        9223,
			Headless:         true,
			MaxSummaryLength: 12000,
			Timeout:          60 * time.Second,
		},
		Storage: StorageConfig{
			SQLitePath: "./data/agent.db",
		},
		Runtime: RuntimeConfig{
			MaxTasksPerProject: 50,
		},
		Communication: CommunicationConfig{
			ChannelID: "cli",
			SessionID: "local",
		},
	}
}

func (c Config) ValidateForLLM() error {
	if strings.TrimSpace(c.LLM.Provider) == "" {
		return errors.New("llm provider is required")
	}
	if strings.TrimSpace(c.LLM.APIKey) == "" {
		return errors.New("llm api key is required; set AGENT_GOGO_LLM_API_KEY, provider-specific env key, or llm.api_key")
	}
	if strings.TrimSpace(c.LLM.Model) == "" {
		return errors.New("llm model is required")
	}
	return nil
}

func applyYAMLFile(cfg *Config, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var section string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "- ") {
			continue
		}
		if !strings.HasPrefix(raw, " ") && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := expandValue(parts[1])
		applyKeyValue(cfg, section, key, value)
	}
	return scanner.Err()
}

func applyKeyValue(cfg *Config, section string, key string, value string) {
	switch section {
	case "llm":
		switch key {
		case "provider":
			cfg.LLM.Provider = value
		case "model":
			cfg.LLM.Model = value
		case "base_url":
			cfg.LLM.BaseURL = value
		case "api_key":
			cfg.LLM.APIKey = value
		case "timeout":
			if seconds, ok := parsePositiveInt(value); ok {
				cfg.LLM.Timeout = time.Duration(seconds) * time.Second
			}
		case "thinking_enabled":
			cfg.LLM.ThinkingEnabled = parseBool(value)
		case "reasoning_effort":
			cfg.LLM.ReasoningEffort = value
		}
	case "browser":
		switch key {
		case "provider":
			cfg.Browser.Provider = value
		case "mcp_url":
			cfg.Browser.MCPURL = value
		case "auto_start_mcp":
			cfg.Browser.AutoStartMCP = parseBool(value)
		case "debug_port":
			if port, ok := parsePositiveInt(value); ok {
				cfg.Browser.DebugPort = port
			}
		case "chrome_path":
			cfg.Browser.ChromePath = value
		case "user_data_dir":
			cfg.Browser.UserDataDir = value
		case "headless":
			cfg.Browser.Headless = parseBool(value)
		case "timeout":
			if seconds, ok := parsePositiveInt(value); ok {
				cfg.Browser.Timeout = time.Duration(seconds) * time.Second
			}
		case "max_summary_length":
			if length, ok := parsePositiveInt(value); ok {
				cfg.Browser.MaxSummaryLength = length
			}
		}
	case "storage":
		if key == "sqlite_path" {
			cfg.Storage.SQLitePath = value
		}
	case "runtime":
		if key == "max_tasks_per_project" {
			if maxTasks, ok := parsePositiveInt(value); ok {
				cfg.Runtime.MaxTasksPerProject = maxTasks
			}
		}
	case "communication":
		switch key {
		case "channel_id":
			cfg.Communication.ChannelID = value
		case "session_id":
			cfg.Communication.SessionID = value
		}
	}
}

func applyEnv(cfg *Config) {
	if value := os.Getenv("AGENT_GOGO_LLM_API_KEY"); value != "" {
		cfg.LLM.APIKey = value
	}
	if value := os.Getenv("DEEPSEEK_API_KEY"); value != "" {
		cfg.LLM.APIKey = value
	}
	if strings.EqualFold(cfg.LLM.Provider, "openai") {
		if value := os.Getenv("OPENAI_API_KEY"); value != "" {
			cfg.LLM.APIKey = value
		}
	}
	if value := os.Getenv("AGENT_GOGO_LLM_PROVIDER"); value != "" {
		cfg.LLM.Provider = value
	}
	if value := os.Getenv("AGENT_GOGO_LLM_MODEL"); value != "" {
		cfg.LLM.Model = value
	}
	if value := os.Getenv("AGENT_GOGO_LLM_BASE_URL"); value != "" {
		cfg.LLM.BaseURL = value
	}
	if value := os.Getenv("AGENT_GOGO_SQLITE_PATH"); value != "" {
		cfg.Storage.SQLitePath = value
	}
	if value := os.Getenv("AGENT_GOGO_CHANNEL_ID"); value != "" {
		cfg.Communication.ChannelID = value
	}
	if value := os.Getenv("AGENT_GOGO_SESSION_ID"); value != "" {
		cfg.Communication.SessionID = value
	}
	if value := os.Getenv("AGENT_GOGO_BROWSER_PROVIDER"); value != "" {
		cfg.Browser.Provider = value
	}
	if value := os.Getenv("AGENT_GOGO_BROWSER_MCP_URL"); value != "" {
		cfg.Browser.MCPURL = value
	}
	if value := os.Getenv("AGENT_GOGO_BROWSER_AUTO_START_MCP"); value != "" {
		cfg.Browser.AutoStartMCP = parseBool(value)
	}
	if value := os.Getenv("AGENT_GOGO_BROWSER_DEBUG_PORT"); value != "" {
		if port, ok := parsePositiveInt(value); ok {
			cfg.Browser.DebugPort = port
		}
	}
	if value := os.Getenv("AGENT_GOGO_BROWSER_CHROME_PATH"); value != "" {
		cfg.Browser.ChromePath = value
	}
	if value := os.Getenv("AGENT_GOGO_BROWSER_USER_DATA_DIR"); value != "" {
		cfg.Browser.UserDataDir = value
	}
}

func existingConfigPath() string {
	for _, candidate := range []string{"config.yaml", filepath.Join("configs", "config.yaml")} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func expandValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return os.ExpandEnv(value)
}

func parsePositiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
