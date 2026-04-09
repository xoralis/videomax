package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"video-max/internal/domain/dto"
	"video-max/internal/domain/entity"
	"video-max/internal/queue"
	"video-max/internal/repository"
	"video-max/pkg/logger"
)

// VideoHandler 视频生成相关的 HTTP 接口控制器
// 只负责接收和校验请求，然后投递到 Kafka，不包含任何业务逻辑
type VideoHandler struct {
	taskRepo  repository.TaskRepository
	producer  *queue.Producer
	uploadDir string // 用户上传图片的本地存储目录
}

// NewVideoHandler 创建视频 Handler 实例
func NewVideoHandler(repo repository.TaskRepository, producer *queue.Producer, uploadDir string) *VideoHandler {
	return &VideoHandler{
		taskRepo:  repo,
		producer:  producer,
		uploadDir: uploadDir,
	}
}

// CreateVideo 处理视频生成请求
// POST /api/video
// 接收 multipart/form-data 格式的图文数据
func (h *VideoHandler) CreateVideo(c *gin.Context) {
	// 1. 绑定并校验表单参数
	var req dto.VideoCreateRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.VideoCreateResponse{
			Code: -1,
			Msg:  fmt.Sprintf("参数校验失败: %s", err.Error()),
		})
		return
	}

	// 2. 处理上传的图片文件（multipart 文件域名为 "images"）
	form, _ := c.MultipartForm()
	var savedPaths []string

	if form != nil && form.File["images"] != nil {
		// 确保上传目录存在
		taskUploadDir := filepath.Join(h.uploadDir, uuid.New().String())
		if err := os.MkdirAll(taskUploadDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, dto.VideoCreateResponse{
				Code: -1,
				Msg:  fmt.Sprintf("创建上传目录失败: %s", err.Error()),
			})
			return
		}

		for i, file := range form.File["images"] {
			// 保存文件到本地目录
			dst := filepath.Join(taskUploadDir, fmt.Sprintf("ref_%d%s", i+1, filepath.Ext(file.Filename)))
			if err := c.SaveUploadedFile(file, dst); err != nil {
				c.JSON(http.StatusInternalServerError, dto.VideoCreateResponse{
					Code: -1,
					Msg:  fmt.Sprintf("保存第 %d 张图片失败: %s", i+1, err.Error()),
				})
				return
			}
			savedPaths = append(savedPaths, dst)
		}
	}

	// 3. 生成任务 ID 并创建数据库记录
	taskID := uuid.New().String()
	taskType := "t2v" // 默认纯文生视频
	if len(savedPaths) > 0 {
		taskType = "i2v" // 有参考图则为图生视频
	}

	task := &entity.Task{
		ID:           taskID,
		Type:         taskType,
		OriginalIdea: req.Idea,
		InputImages:  fmt.Sprintf("%v", savedPaths), // 简单序列化路径列表
		Status:       entity.TaskStatusPending,
	}

	if err := h.taskRepo.Create(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, dto.VideoCreateResponse{
			Code: -1,
			Msg:  fmt.Sprintf("创建任务失败: %s", err.Error()),
		})
		return
	}

	// 4. 投递到 Kafka 异步处理
	kafkaMsg := queue.VideoTaskMessage{
		TaskID:      taskID,
		UserIdea:    req.Idea,
		ImagePaths:  savedPaths,
		AspectRatio: req.AspectRatio,
	}

	if err := h.producer.PublishVideoTask(kafkaMsg); err != nil {
		logger.Log.Errorw("投递 Kafka 消息失败", "task_id", taskID, "error", err)
		// Kafka 投递失败不影响任务创建，后续可通过补偿机制重试
	}

	// 5. 立即返回任务 ID，不阻塞等待视频生成
	c.JSON(http.StatusOK, dto.VideoCreateResponse{
		Code:   0,
		TaskID: taskID,
		Msg:    "任务已提交，请使用 task_id 轮询进度",
	})

	logger.Log.Infow("视频生成任务创建成功",
		"task_id", taskID,
		"type", taskType,
		"image_count", len(savedPaths),
	)
}

// QueryTask 查询任务状态
// GET /api/task/:id
func (h *VideoHandler) QueryTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, dto.TaskQueryResponse{
			Code: -1,
			Msg:  "任务 ID 不能为空",
		})
		return
	}

	task, err := h.taskRepo.GetByID(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.TaskQueryResponse{
			Code: -1,
			Msg:  fmt.Sprintf("任务不存在: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, dto.TaskQueryResponse{
		Code:     0,
		TaskID:   task.ID,
		Status:   task.Status,
		VideoURL: task.VideoURL,
		Msg:      "查询成功",
	})
}
