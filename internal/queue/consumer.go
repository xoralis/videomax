package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"video-max/internal/domain/entity"
	"video-max/internal/mas"
	"video-max/internal/repository"
	"video-max/internal/video"
	"video-max/pkg/logger"
	ossuploader "video-max/pkg/oss"
)

// Consumer Kafka 消费者，负责从 Topic 中取出任务并驱动 MAS 多智能体流水线
type Consumer struct {
	orchestrator *mas.Orchestrator         // 多智能体调度器
	taskRepo     repository.TaskRepository // 任务存储层
	videoFactory *video.VideoFactory       // 视频生成服务工厂（支持按模型选择 Provider）
	emitter      *mas.EventEmitter         // SSE 事件发射器
	ossUploader  *ossuploader.Uploader     // 阿里云 OSS 上传器（可选，nil 时降级使用原始 URL）
}

// NewConsumer 创建 Kafka 消费者实例
func NewConsumer(orch *mas.Orchestrator, repo repository.TaskRepository, vf *video.VideoFactory, emitter *mas.EventEmitter, uploader *ossuploader.Uploader) *Consumer {
	return &Consumer{
		orchestrator: orch,
		taskRepo:     repo,
		videoFactory: vf,
		emitter:      emitter,
		ossUploader:  uploader,
	}
}

// Setup 实现 sarama.ConsumerGroupHandler 接口 - 消费者启动时调用
func (c *Consumer) Setup(_ sarama.ConsumerGroupSession) error {
	logger.Log.Info("Kafka Consumer: 消费者组会话已建立")
	return nil
}

// Cleanup 实现 sarama.ConsumerGroupHandler 接口 - 消费者关闭时调用
func (c *Consumer) Cleanup(_ sarama.ConsumerGroupSession) error {
	logger.Log.Info("Kafka Consumer: 消费者组会话已清理")
	return nil
}

// ConsumeClaim 实现 sarama.ConsumerGroupHandler 接口 - 核心消费逻辑
// 每收到一条 Kafka 消息，就启动一次完整的 MAS 流水线处理
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		logger.Log.Infow("Kafka Consumer: 收到新消息",
			"topic", msg.Topic,
			"partition", msg.Partition,
			"offset", msg.Offset,
		)

		// 解析消息体
		var taskMsg VideoTaskMessage
		if err := json.Unmarshal(msg.Value, &taskMsg); err != nil {
			logger.Log.Errorw("Kafka Consumer: 消息解析失败，跳过",
				"error", err,
				"raw", string(msg.Value),
			)
			session.MarkMessage(msg, "")
			continue
		}

		// 处理任务（在一个独立的方法中执行，方便错误追踪）
		if err := c.processTask(session.Context(), taskMsg); err != nil {
			logger.Log.Errorw("Kafka Consumer: 任务处理失败",
				"task_id", taskMsg.TaskID,
				"error", err,
			)
			// 标记任务为失败状态
			_ = c.taskRepo.MarkFailed(session.Context(), taskMsg.TaskID, err.Error())
		}

		// 标记消息已消费（无论成功失败都要标记，避免重复消费）
		session.MarkMessage(msg, "")
	}
	return nil
}

// processTask 处理一个完整的视频生成任务
// 执行流程: 更新状态 -> MAS多智能体协作 -> 保存提示词 -> 提交视频生成 -> 轮询结果 -> 保存下载链接
func (c *Consumer) processTask(ctx context.Context, msg VideoTaskMessage) error {
	taskID := msg.TaskID

	// 根 Span：覆盖整个任务生命周期（MAS 流水线 + 视频生成）
	ctx, rootSpan := otel.Tracer("videomax").Start(ctx, "Task-"+taskID,
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "chain"),
			attribute.String("task.id", taskID),
			attribute.String("user.idea", msg.UserIdea),
			attribute.String("video.aspect_ratio", msg.AspectRatio),
			attribute.Int("video.image_count", len(msg.ImagePaths)),
		))
	defer rootSpan.End()

	// 任务完成（无论成功/失败）后关闭 SSE 事件通道，通知前端连接结束
	defer c.emitter.Close(taskID)

	// 1. 更新任务状态为「处理中」
	if err := c.taskRepo.UpdateStatus(ctx, taskID, entity.TaskStatusStory); err != nil {
		return fmt.Errorf("更新任务状态失败: %w", err)
	}

	// 2. 启动 MAS 多智能体协作流水线
	logger.Log.Infow("开始 MAS 多智能体协作", "task_id", taskID)
	masResult, err := c.orchestrator.Run(ctx, taskID, msg.UserIdea, msg.ImagePaths, msg.AspectRatio)
	if err != nil {
		return fmt.Errorf("MAS 协作失败: %w", err)
	}

	// 3. 保存 Agent 链产出的最终提示词
	if err := c.taskRepo.SaveEnhancedPrompt(ctx, taskID, masResult.FinalPrompts); err != nil {
		return fmt.Errorf("保存增强提示词失败: %w", err)
	}

	// 4. 更新状态为「生成中」，并按模型名称选取视频 Provider 提交任务
	if err := c.taskRepo.UpdateStatus(ctx, taskID, entity.TaskStatusGenerating); err != nil {
		return fmt.Errorf("更新任务状态失败: %w", err)
	}

	provider, err := c.videoFactory.GetProvider(msg.Model)
	if err != nil {
		return fmt.Errorf("获取视频Provider失败: %w", err)
	}

	_, submitSpan := otel.Tracer("videomax").Start(ctx, "VideoGeneration.Submit",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "tool"),
			attribute.String("video.prompt", masResult.FinalPrompts),
			attribute.Int("video.image_count", len(msg.ImagePaths)),
		))
	genResult, err := provider.GenerateVideo(ctx, video.GenerateRequest{
		Prompt:      masResult.FinalPrompts,
		ImagePaths:  msg.ImagePaths,
		AspectRatio: msg.AspectRatio,
		Duration:    msg.Duration,
	})
	if err != nil {
		submitSpan.RecordError(err)
		submitSpan.SetStatus(codes.Error, err.Error())
		submitSpan.End()
		return fmt.Errorf("提交视频生成请求失败: %w", err)
	}
	submitSpan.SetAttributes(attribute.String("video.provider_task_id", genResult.ProviderTaskID))
	submitSpan.End()

	// 5. 轮询视频生成状态（简单实现，后续可优化为 Webhook 回调）
	logger.Log.Infow("开始轮询视频生成状态",
		"task_id", taskID,
		"provider_task_id", genResult.ProviderTaskID,
	)

	_, pollSpan := otel.Tracer("videomax").Start(ctx, "VideoGeneration.Poll",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "tool"),
			attribute.String("video.provider_task_id", genResult.ProviderTaskID),
		))
	defer pollSpan.End()

	status, err := c.pollVideoStatus(ctx, provider, genResult.ProviderTaskID)
	if err != nil {
		pollSpan.RecordError(err)
		pollSpan.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("轮询视频状态失败: %w", err)
	}
	if !status.IsFailed {
		pollSpan.SetAttributes(attribute.String("video.url", status.VideoURL))
	}

	if status.IsFailed {
		return fmt.Errorf("视频生成失败: %s", status.ErrorMsg)
	}

	// 6. 可选：将视频上传到阿里云 OSS，替换临时 CDN 链接为永久 OSS URL
	finalURL := status.VideoURL
	if c.ossUploader != nil {
		objectKey := fmt.Sprintf("videos/%s.mp4", taskID)
		ossURL, uploadErr := c.ossUploader.UploadFromURL(ctx, status.VideoURL, objectKey)
		if uploadErr != nil {
			// OSS 上传失败不阻断主流程，降级保留原始 CDN URL
			logger.Log.Warnw("OSS 上传失败，降级使用原始视频 URL",
				"task_id", taskID,
				"error", uploadErr,
			)
		} else {
			finalURL = ossURL
			logger.Log.Infow("视频已上传至 OSS", "task_id", taskID, "oss_url", ossURL)
		}
	}

	// 7. 保存最终结果（OSS URL 或原始 CDN URL）
	if err := c.taskRepo.SaveResult(ctx, taskID, finalURL, genResult.ProviderTaskID); err != nil {
		return fmt.Errorf("保存视频结果失败: %w", err)
	}

	logger.Log.Infow("🎉 视频生成任务完成！",
		"task_id", taskID,
		"video_url", finalURL,
	)
	return nil
}

// pollVideoStatus 轮询视频生成状态
func (c *Consumer) pollVideoStatus(ctx context.Context, provider video.VideoProvider, providerTaskID string) (*video.TaskStatus, error) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	timeout := time.After(10 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("轮询超时（10分钟）")
		case <-ticker.C:
			status, err := provider.CheckStatus(ctx, providerTaskID)
			if err != nil {
				logger.Log.Warnw("轮询查询失败，稍后重试", "error", err)
				continue
			}
			if status.IsFinished {
				return status, nil
			}
			logger.Log.Infow("视频生成中，继续等待...", "provider_task_id", providerTaskID)
		}
	}
}
