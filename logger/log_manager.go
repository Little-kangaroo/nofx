package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// LogManager 日志管理器
type LogManager struct {
	logFile   *os.File
	logDir    string
	maxSizeMB int64
	maxDays   int
}

// LogConfig 日志配置
type LogConfig struct {
	LogDir    string // 日志目录，默认 "logs"
	MaxSizeMB int64  // 单个日志文件最大大小(MB)，默认 50MB
	MaxDays   int    // 日志保留天数，默认 7天
}

// defaultLogConfig 默认日志配置
var defaultLogConfig = LogConfig{
	LogDir:    "logs",
	MaxSizeMB: 50,
	MaxDays:   7,
}

// NewLogManager 创建日志管理器
func NewLogManager(config ...LogConfig) (*LogManager, error) {
	cfg := defaultLogConfig
	if len(config) > 0 {
		cfg = config[0]
		// 设置默认值
		if cfg.LogDir == "" {
			cfg.LogDir = defaultLogConfig.LogDir
		}
		if cfg.MaxSizeMB <= 0 {
			cfg.MaxSizeMB = defaultLogConfig.MaxSizeMB
		}
		if cfg.MaxDays <= 0 {
			cfg.MaxDays = defaultLogConfig.MaxDays
		}
	}
	
	lm := &LogManager{
		logDir:    cfg.LogDir,
		maxSizeMB: cfg.MaxSizeMB,
		maxDays:   cfg.MaxDays,
	}
	
	// 创建日志目录
	if err := os.MkdirAll(lm.logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	
	// 初始化日志文件
	if err := lm.initLogFile(); err != nil {
		return nil, fmt.Errorf("初始化日志文件失败: %w", err)
	}
	
	// 清理旧日志文件
	go lm.cleanupOldLogs()
	
	log.Printf("✅ 日志系统初始化成功: 目录=%s, 最大大小=%dMB, 保留=%d天", 
		lm.logDir, lm.maxSizeMB, lm.maxDays)
	
	return lm, nil
}

// initLogFile 初始化日志文件
func (lm *LogManager) initLogFile() error {
	// 生成今天的日志文件名
	today := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("nofx_%s.log", today)
	filepath := filepath.Join(lm.logDir, filename)
	
	// 打开或创建日志文件
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	
	lm.logFile = file
	
	// 设置log输出到文件和标准输出
	multiWriter := io.MultiWriter(os.Stdout, file)
	log.SetOutput(multiWriter)
	
	// 设置日志格式，包含时间戳
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	return nil
}

// rotateLogIfNeeded 如果需要则轮转日志
func (lm *LogManager) rotateLogIfNeeded() error {
	if lm.logFile == nil {
		return nil
	}
	
	// 检查文件大小
	fileInfo, err := lm.logFile.Stat()
	if err != nil {
		return err
	}
	
	// 检查是否需要按大小轮转
	if fileInfo.Size() > lm.maxSizeMB*1024*1024 {
		log.Printf("📋 日志文件大小达到%dMB，开始轮转", lm.maxSizeMB)
		return lm.rotateLogs()
	}
	
	// 检查是否需要按日期轮转（每天一个文件）
	today := time.Now().Format("2006-01-02")
	expectedFilename := fmt.Sprintf("nofx_%s.log", today)
	if fileInfo.Name() != expectedFilename {
		log.Printf("📋 日期变更，开始日志轮转")
		return lm.rotateLogs()
	}
	
	return nil
}

// rotateLogs 轮转日志文件
func (lm *LogManager) rotateLogs() error {
	// 关闭当前日志文件
	if lm.logFile != nil {
		lm.logFile.Close()
	}
	
	// 重新初始化日志文件
	return lm.initLogFile()
}

// cleanupOldLogs 清理旧日志文件
func (lm *LogManager) cleanupOldLogs() {
	ticker := time.NewTicker(24 * time.Hour) // 每24小时清理一次
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			lm.performCleanup()
		}
	}
}

// performCleanup 执行清理
func (lm *LogManager) performCleanup() {
	cutoffTime := time.Now().AddDate(0, 0, -lm.maxDays)
	
	err := filepath.Walk(lm.logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略错误，继续处理
		}
		
		// 只处理日志文件
		if filepath.Ext(info.Name()) == ".log" && info.ModTime().Before(cutoffTime) {
			if removeErr := os.Remove(path); removeErr != nil {
				log.Printf("⚠️ 删除旧日志文件失败: %s, 错误: %v", path, removeErr)
			} else {
				log.Printf("🗑️ 已删除旧日志文件: %s", info.Name())
			}
		}
		return nil
	})
	
	if err != nil {
		log.Printf("⚠️ 清理日志文件时发生错误: %v", err)
	}
}

// Close 关闭日志管理器
func (lm *LogManager) Close() error {
	if lm.logFile != nil {
		log.Printf("📋 正在关闭日志系统...")
		return lm.logFile.Close()
	}
	return nil
}

// GetLogFile 获取当前日志文件路径
func (lm *LogManager) GetLogFile() string {
	if lm.logFile == nil {
		return ""
	}
	return lm.logFile.Name()
}

// CheckAndRotate 检查并轮转日志（供外部调用）
func (lm *LogManager) CheckAndRotate() error {
	return lm.rotateLogIfNeeded()
}

// LogWithLevel 带级别的日志记录
func LogInfo(message string, args ...interface{}) {
	log.Printf("ℹ️ [INFO] "+message, args...)
}

func LogError(message string, args ...interface{}) {
	log.Printf("❌ [ERROR] "+message, args...)
}

func LogWarning(message string, args ...interface{}) {
	log.Printf("⚠️ [WARN] "+message, args...)
}

func LogDebug(message string, args ...interface{}) {
	log.Printf("🔍 [DEBUG] "+message, args...)
}

func LogSuccess(message string, args ...interface{}) {
	log.Printf("✅ [SUCCESS] "+message, args...)
}

func LogTrading(message string, args ...interface{}) {
	log.Printf("💰 [TRADING] "+message, args...)
}

func LogSystemEvent(message string, args ...interface{}) {
	log.Printf("🔧 [SYSTEM] "+message, args...)
}