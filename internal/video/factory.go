package video

import (
	"fmt"

	"video-max/pkg/config"
)

// 支持的视频服务提供商标识常量
const (
	ProviderByteDance = "bytedance"
	ProviderKling     = "kling"
)

// VideoFactory 持有全部已注册的视频生成 Provider
// key 为模型名称（如 "doubao-seedance-1-0-pro-250528"），value 为对应的 VideoProvider 实例
type VideoFactory struct {
	providers map[string]VideoProvider
}

// NewVideoFactory 根据配置列表初始化所有视频 Provider
// 每个 Provider 以其模型名称为 key 注册到工厂中，供后续 per-request 按 model 选取
func NewVideoFactory(cfgs []config.VideoProviderConfig) (*VideoFactory, error) {
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("video providers 配置为空，至少需要配置一个视频服务商")
	}
	providers := make(map[string]VideoProvider, len(cfgs))
	for _, c := range cfgs {
		p, err := newVideoProvider(c.Provider, c.APIKey, c.BaseURL, c.Name)
		if err != nil {
			return nil, fmt.Errorf("初始化视频Provider [%s] 失败: %w", c.Name, err)
		}
		providers[c.Name] = p
	}
	return &VideoFactory{providers: providers}, nil
}

// GetProvider 根据模型名称选取对应的 VideoProvider
// 若未找到对应 Provider，返回错误
func (f *VideoFactory) GetProvider(model string) (VideoProvider, error) {
	p, ok := f.providers[model]
	if !ok {
		return nil, fmt.Errorf("未找到模型 [%s] 对应的视频Provider，请检查配置", model)
	}
	return p, nil
}

// newVideoProvider 内部工厂函数，根据供应商标识创建 VideoProvider 实例
func newVideoProvider(providerName string, apiKey string, baseURL string, model string) (VideoProvider, error) {
	switch providerName {
	case ProviderByteDance:
		return NewByteDanceClient(apiKey, baseURL, model), nil
	case ProviderKling:
		return NewKlingClient(apiKey, baseURL, model), nil
	default:
		return nil, fmt.Errorf("不支持的视频服务提供商: %s，当前支持: %s, %s", providerName, ProviderByteDance, ProviderKling)
	}
}
