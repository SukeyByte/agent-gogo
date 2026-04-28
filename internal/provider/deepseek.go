package provider

const (
	DefaultDeepSeekBaseURL = "https://api.deepseek.com"
	DefaultDeepSeekModel   = "deepseek-v4-flash"
)

type DeepSeekConfig = OpenAICompatibleConfig
type DeepSeekProvider = OpenAICompatibleProvider

func NewDeepSeekProvider(config DeepSeekConfig) (*DeepSeekProvider, error) {
	if config.ProviderName == "" {
		config.ProviderName = "deepseek"
	}
	if config.DefaultBaseURL == "" {
		config.DefaultBaseURL = DefaultDeepSeekBaseURL
	}
	if config.DefaultChatModel == "" {
		config.DefaultChatModel = DefaultDeepSeekModel
	}
	return NewOpenAICompatibleProvider(config)
}
