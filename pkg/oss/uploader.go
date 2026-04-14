package oss

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	alioss "github.com/aliyun/aliyun-oss-go-sdk/oss"

	"video-max/pkg/config"
)

// Uploader 阿里云 OSS 上传工具
// 将远程视频 URL 流式下载后直接上传到 OSS，无需本地落盘
type Uploader struct {
	bucket  *alioss.Bucket
	baseURL string // 可选：自定义 CDN 域名，如 https://cdn.example.com
	cfg     config.OSSConfig
}

// NewUploader 根据配置初始化 OSS Uploader
func NewUploader(cfg config.OSSConfig) (*Uploader, error) {
	client, err := alioss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("初始化 OSS 客户端失败: %w", err)
	}

	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("获取 OSS Bucket 失败: %w", err)
	}

	return &Uploader{
		bucket:  bucket,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		cfg:     cfg,
	}, nil
}

// UploadFromURL 从远程 URL 流式下载视频并上传到 OSS
// objectKey: OSS 中的存储路径，如 "videos/abc123.mp4"
// 返回: 上传成功后的 OSS 访问 URL
func (u *Uploader) UploadFromURL(ctx context.Context, videoURL, objectKey string) (string, error) {
	// 带超时的 HTTP 客户端下载视频流
	httpClient := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建下载请求失败: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载视频失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载视频返回非 200 状态码: %d", resp.StatusCode)
	}

	// 流式上传到 OSS（响应 body 直接作为 reader，无需落盘）
	if err := u.bucket.PutObject(objectKey, resp.Body,
		alioss.ContentType("video/mp4"),
	); err != nil {
		return "", fmt.Errorf("上传到 OSS 失败: %w", err)
	}

	return u.buildURL(objectKey), nil
}

// buildURL 根据配置生成 OSS 访问 URL
func (u *Uploader) buildURL(objectKey string) string {
	if u.baseURL != "" {
		return fmt.Sprintf("%s/%s", u.baseURL, objectKey)
	}
	// 默认 OSS 地址格式：https://{bucket}.{endpoint}/{key}
	return fmt.Sprintf("https://%s.%s/%s", u.cfg.Bucket, u.cfg.Endpoint, objectKey)
}
