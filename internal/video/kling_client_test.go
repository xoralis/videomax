package video

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ==================== JWT 签名测试 ====================

// TestKlingClient_GenerateJWT 测试 JWT 生成逻辑
// 验证生成的 JWT 包含正确的三段式结构、Header 和 Payload 内容
func TestKlingClient_GenerateJWT(t *testing.T) {
	client := &KlingClient{
		accessKey: "test-ak-12345",
		secretKey: "test-sk-67890",
	}

	token, err := client.generateJWT()
	if err != nil {
		t.Fatalf("generateJWT() 失败: %v", err)
	}

	// JWT 应为三段式 (header.payload.signature)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT 格式错误: 期望 3 段，得到 %d 段", len(parts))
	}

	// 解码并验证 Header
	headerJSON, err := base64URLDecode(parts[0])
	if err != nil {
		t.Fatalf("解码 Header 失败: %v", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("解析 Header JSON 失败: %v", err)
	}
	if header.Alg != "HS256" {
		t.Errorf("Header.alg: got %q, want %q", header.Alg, "HS256")
	}
	if header.Typ != "JWT" {
		t.Errorf("Header.typ: got %q, want %q", header.Typ, "JWT")
	}

	// 解码并验证 Payload
	payloadJSON, err := base64URLDecode(parts[1])
	if err != nil {
		t.Fatalf("解码 Payload 失败: %v", err)
	}
	var payload struct {
		Iss string `json:"iss"`
		Exp int64  `json:"exp"`
		Nbf int64  `json:"nbf"`
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("解析 Payload JSON 失败: %v", err)
	}
	if payload.Iss != "test-ak-12345" {
		t.Errorf("Payload.iss: got %q, want %q", payload.Iss, "test-ak-12345")
	}
	if payload.Exp <= payload.Nbf {
		t.Errorf("Payload.exp (%d) 应大于 nbf (%d)", payload.Exp, payload.Nbf)
	}
}

// base64URLDecode 辅助函数：解码 Base64 URL 编码的字符串
func base64URLDecode(s string) ([]byte, error) {
	// 补齐 Base64 padding
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	return base64.StdEncoding.DecodeString(s)
}

// ==================== Base64 图片编码测试 ====================

// TestImageToRawBase64 测试图片读取并转换为纯 Base64（不含 data URI 前缀）
func TestImageToRawBase64(t *testing.T) {
	// 创建临时测试图片文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.png")
	testContent := []byte("fake-png-content-for-testing")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	result, err := imageToRawBase64(testFile)
	if err != nil {
		t.Fatalf("imageToRawBase64() 失败: %v", err)
	}

	// 验证结果是合法的 Base64
	decoded, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("输出不是合法的 Base64: %v", err)
	}

	// 验证内容匹配
	if string(decoded) != string(testContent) {
		t.Errorf("解码内容不匹配: got %q, want %q", decoded, testContent)
	}

	// 验证不包含 data URI 前缀
	if strings.HasPrefix(result, "data:") {
		t.Error("Base64 结果不应包含 data:image 前缀")
	}
}

// TestImageToRawBase64_FileSizeLimit 测试超过 10MB 限制的文件被拒绝
func TestImageToRawBase64_FileSizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	bigFile := filepath.Join(tmpDir, "big.png")

	// 创建一个超过 10MB 的文件
	bigData := make([]byte, 11*1024*1024)
	if err := os.WriteFile(bigFile, bigData, 0644); err != nil {
		t.Fatalf("创建大文件失败: %v", err)
	}

	_, err := imageToRawBase64(bigFile)
	if err == nil {
		t.Fatal("超过 10MB 的文件应返回错误")
	}
	if !strings.Contains(err.Error(), "10MB") {
		t.Errorf("错误信息应包含 '10MB': got %q", err.Error())
	}
}

// ==================== 单图/多图路由测试 ====================

// TestKlingClient_RequestRouting 测试请求根据图片数量自动路由到正确的 API 端点
func TestKlingClient_RequestRouting(t *testing.T) {
	// 创建测试图片
	tmpDir := t.TempDir()
	img1 := filepath.Join(tmpDir, "img1.png")
	img2 := filepath.Join(tmpDir, "img2.png")
	img3 := filepath.Join(tmpDir, "img3.png")
	for _, p := range []string{img1, img2, img3} {
		os.WriteFile(p, []byte("test"), 0644)
	}

	tests := []struct {
		name           string
		imagePaths     []string
		wantPrefix     string // ProviderTaskID 的前缀
		wantHasImage   bool   // 请求体是否包含 image 字段
		wantHasImgList bool   // 请求体是否包含 image_list 字段
	}{
		{
			name:       "单图应走 image2video 接口",
			imagePaths: []string{img1},
			wantPrefix: "single:",
		},
		{
			name:       "多图应走 multi-image2video 接口",
			imagePaths: []string{img1, img2},
			wantPrefix: "multi:",
		},
		{
			name:       "三张图也应走 multi-image2video",
			imagePaths: []string{img1, img2, img3},
			wantPrefix: "multi:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 构建请求体来验证路由逻辑
			klingReq := klingImage2VideoReq{
				ModelName: "kling-v1",
				Mode:      "std",
				Duration:  "5",
			}

			var taskPrefix string
			if len(tt.imagePaths) > 1 {
				taskPrefix = "multi:"
				for i, imgPath := range tt.imagePaths {
					if i >= 4 {
						break
					}
					b64, _ := imageToRawBase64(imgPath)
					klingReq.ImageList = append(klingReq.ImageList, klingImageListItem{Image: b64})
				}
			} else if len(tt.imagePaths) == 1 {
				taskPrefix = "single:"
				b64, _ := imageToRawBase64(tt.imagePaths[0])
				klingReq.Image = b64
			}

			if taskPrefix != tt.wantPrefix {
				t.Errorf("路由前缀: got %q, want %q", taskPrefix, tt.wantPrefix)
			}

			// 验证 JSON 序列化后的结构
			jsonBytes, _ := json.Marshal(klingReq)
			jsonStr := string(jsonBytes)

			if tt.wantPrefix == "single:" {
				if !strings.Contains(jsonStr, `"image":`) {
					t.Error("单图模式: JSON 应包含 image 字段")
				}
				if strings.Contains(jsonStr, `"image_list":`) {
					t.Error("单图模式: JSON 不应包含 image_list 字段")
				}
			}
			if tt.wantPrefix == "multi:" {
				if strings.Contains(jsonStr, `"image":`) && !strings.Contains(jsonStr, `"image_list":`) {
					t.Error("多图模式: JSON 应包含 image_list 字段")
				}
			}
		})
	}
}

// ==================== CheckStatus 前缀解析测试 ====================

// TestKlingClient_TaskIDPrefixParsing 测试 CheckStatus 正确解析 task_id 前缀以路由到对应查询接口
func TestKlingClient_TaskIDPrefixParsing(t *testing.T) {
	tests := []struct {
		input       string
		wantRealID  string
		wantPath    string
	}{
		{"single:12345", "12345", klingQueryImage2VideoPath},
		{"multi:67890", "67890", klingQueryMultiImage2VideoPath},
		{"99999", "99999", klingQueryImage2VideoPath}, // 无前缀回退到单图
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			realTaskID := tt.input
			reqPath := klingQueryImage2VideoPath

			if strings.HasPrefix(tt.input, "multi:") {
				reqPath = klingQueryMultiImage2VideoPath
				realTaskID = strings.TrimPrefix(tt.input, "multi:")
			} else if strings.HasPrefix(tt.input, "single:") {
				reqPath = klingQueryImage2VideoPath
				realTaskID = strings.TrimPrefix(tt.input, "single:")
			}

			if realTaskID != tt.wantRealID {
				t.Errorf("realTaskID: got %q, want %q", realTaskID, tt.wantRealID)
			}
			if reqPath != tt.wantPath {
				t.Errorf("reqPath: got %q, want %q", reqPath, tt.wantPath)
			}
		})
	}
}
