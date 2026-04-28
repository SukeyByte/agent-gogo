package provider

const (
	DefaultOpenAIBaseURL = "https://api.openai.com/v1"
)

type OpenAIConfig = OpenAICompatibleConfig
type OpenAIProvider = OpenAICompatibleProvider

func NewOpenAIProvider(config OpenAIConfig) (*OpenAIProvider, error) {
	if config.ProviderName == "" {
		config.ProviderName = "openai"
	}
	if config.DefaultBaseURL == "" {
		config.DefaultBaseURL = DefaultOpenAIBaseURL
	}
	return NewOpenAICompatibleProvider(config)
}
