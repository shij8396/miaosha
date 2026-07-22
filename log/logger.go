package log

import (
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	globalLogger *zap.Logger
	globalSugar  *zap.SugaredLogger
	once         sync.Once
)

type Config struct {
	Level      string
	Format     string
	Output     string // "stdout", "file", "both"
	FilePath   string
	MaxSize    int  // 单个日志文件最大大小（MB）
	MaxBackups int  // 最多保留的旧日志文件数
	MaxAge     int  // 最多保留天数
	Compress   bool // 是否压缩旧日志
}

func Init(cfg Config) error {
	var initErr error
	once.Do(func() {
		level := zapcore.InfoLevel
		switch cfg.Level {
		case "debug": level = zapcore.DebugLevel
		case "warn": level = zapcore.WarnLevel
		case "error": level = zapcore.ErrorLevel
		}
		encoderCfg := zap.NewProductionEncoderConfig()
		encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
		encoderCfg.EncodeCaller = zapcore.ShortCallerEncoder
		var encoder zapcore.Encoder
		if cfg.Format == "console" { encoder = zapcore.NewConsoleEncoder(encoderCfg) } else { encoder = zapcore.NewJSONEncoder(encoderCfg) }
		var writers []zapcore.WriteSyncer
		switch cfg.Output {
		case "file":
			dir := filepath.Dir(cfg.FilePath)
			_ = os.MkdirAll(dir, 0755)
			lj := &lumberjack.Logger{
				Filename:   cfg.FilePath,
				MaxSize:    cfg.MaxSize,
				MaxBackups: cfg.MaxBackups,
				MaxAge:     cfg.MaxAge,
				Compress:   cfg.Compress,
			}
			if lj.MaxSize == 0 { lj.MaxSize = 100 }
			if lj.MaxBackups == 0 { lj.MaxBackups = 30 }
			if lj.MaxAge == 0 { lj.MaxAge = 7 }
			writers = append(writers, zapcore.AddSync(lj))
		case "both":
			dir := filepath.Dir(cfg.FilePath)
			_ = os.MkdirAll(dir, 0755)
			lj := &lumberjack.Logger{
				Filename:   cfg.FilePath,
				MaxSize:    cfg.MaxSize,
				MaxBackups: cfg.MaxBackups,
				MaxAge:     cfg.MaxAge,
				Compress:   cfg.Compress,
			}
			if lj.MaxSize == 0 { lj.MaxSize = 100 }
			if lj.MaxBackups == 0 { lj.MaxBackups = 30 }
			if lj.MaxAge == 0 { lj.MaxAge = 7 }
			writers = append(writers, zapcore.AddSync(os.Stdout), zapcore.AddSync(lj))
		default:
			writers = append(writers, zapcore.AddSync(os.Stdout))
		}
		var writer zapcore.WriteSyncer
		if len(writers) == 1 { writer = writers[0] } else { writer = zapcore.NewMultiWriteSyncer(writers...) }
		core := zapcore.NewCore(encoder, writer, level)
		globalLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
		globalSugar = globalLogger.Sugar()
	})
	return initErr
}

func L() *zap.SugaredLogger {
	if globalSugar == nil { l, _ := zap.NewDevelopment(); return l.Sugar() }
	return globalSugar
}

func Logger() *zap.Logger {
	if globalLogger == nil { l, _ := zap.NewDevelopment(); return l }
	return globalLogger
}

func Sync() { if globalLogger != nil { _ = globalLogger.Sync() } }

func WithTraceID(traceID string) *zap.SugaredLogger { return L().With("trace_id", traceID) }
func WithContext(fields ...interface{}) *zap.SugaredLogger { return L().With(fields...) }

// [修复] NewGormWriter 创建 GORM 日志 Writer，将慢查询日志输出到 zap
// 实现 gorm.io/gorm/logger.Writer 接口，用于 GORM 的 SlowThreshold 慢查询日志
func NewGormWriter() *gormWriter {
	return &gormWriter{}
}

type gormWriter struct{}

func (w *gormWriter) Printf(format string, args ...interface{}) {
	L().Warnf(format, args...)
}