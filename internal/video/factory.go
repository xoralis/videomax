package video

import "fmt"

// 支持的视频服务提供商标识常量
const (
	ProviderByteDance = "bytedance"
	ProviderKling     = "kling"
)

// NewVideoProvider 视频服务提供商工厂函数
// 根据传入的供应商名称和 API 密钥，创建并返回对应的 VideoProvider 实现
// 新增供应商时，只需在此处添加一个 case 分支，并实现 VideoProvider 接口即可
func NewVideoProvider(providerName string, apiKey string, baseURL string, model string) (VideoProvider, error) {
	switch providerName {
	case ProviderByteDance:
		return NewByteDanceClient(apiKey, baseURL, model), nil
	// 未来扩展示例:
	// case ProviderKling:
	//     return NewKlingClient(apiKey, baseURL, model), nil
	default:
		return nil, fmt.Errorf("不支持的视频服务提供商: %s，当前支持: %s", providerName, ProviderByteDance)
	}
}
