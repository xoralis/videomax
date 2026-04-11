package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"video-max/pkg/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// 全局日志实例，系统中所有模块统一使用此实例输出日志
var Log *zap.SugaredLogger

// Init 初始化全局日志器
// 使用 zap 高性能结构化日志库，支持带颜色的控制台输出
func Init(cfg config.LogConfig) error {
	var level zapcore.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = zapcore.DebugLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	var cores []zapcore.Core

	// 配置编码器：人类可读的控制台格式，带有颜色和时间戳
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // 带颜色的级别标签
		EncodeTime:     zapcore.ISO8601TimeEncoder,       // ISO8601 时间格式
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	if cfg.Mode == "console" || cfg.Mode == "both" {
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			level,
		)
		cores = append(cores, consoleCore)
	}

	if cfg.Mode == "file" || cfg.Mode == "both" {
		if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0755); err != nil {
			return err
		}

		writer := zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		})

		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			writer,
			level,
		)
		cores = append(cores, fileCore)
	}

	if len(cores) == 0 {
		return fmt.Errorf("invalid log mode: %s", cfg.Mode)
	}

	Log = zap.New(zapcore.NewTee(cores...), zap.AddCaller()).Sugar()
	return nil
}

// func Init(level string) error {
// 	switch level {
// 	case "debug":
// 		zapLevel = zapcore.DebugLevel
// 	case "warn":
// 		zapLevel = zapcore.WarnLevel
// 	case "error":
// 		zapLevel = zapcore.ErrorLevel
// 	default:
// 		zapLevel = zapcore.InfoLevel
// 	}
// 配置编码器：人类可读的控制台格式，带有颜色和时间戳
// encoderConfig := zapcore.EncoderConfig{
// 	TimeKey:        "time",
// 	LevelKey:       "level",
// 	NameKey:        "logger",
// 	CallerKey:      "caller",
// 	MessageKey:     "msg",
// 	StacktraceKey:  "stacktrace",
// 	LineEnding:     zapcore.DefaultLineEnding,
// 	EncodeLevel:    zapcore.CapitalColorLevelEncoder, // 带颜色的级别标签
// 	EncodeTime:     zapcore.ISO8601TimeEncoder,       // ISO8601 时间格式
// 	EncodeDuration: zapcore.StringDurationEncoder,
// 	EncodeCaller:   zapcore.ShortCallerEncoder,
// }
// 	core := zapcore.NewCore(
// 		zapcore.NewConsoleEncoder(encoderConfig),
// 		zapcore.AddSync(os.Stdout),
// 		zapLevel,
// 	)

// 	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0))
// 	Log = logger.Sugar()
// }
