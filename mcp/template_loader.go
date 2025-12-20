package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
)

// TemplateBundle 模板包：包含模板内容和SHA-256指纹
type TemplateBundle struct {
	SystemTemplate string // 用于 request.messages[0].content
	HashSHA256     string // 规范化后的 SHA-256 hash
	Bytes          int    // 字节数
	SourcePath     string // 源文件路径
}

// normalizeTemplateBytes 规范化模板字节（最小环境差异规范化）
// - 去除UTF-8 BOM（EF BB BF）
// - CRLF (\r\n) -> LF (\n)
// 不做Trim，不改变语义内容，确保缓存一致性
func normalizeTemplateBytes(b []byte) []byte {
	// 去除UTF-8 BOM: EF BB BF
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
		log.Printf("🔧 [TEMPLATE] 检测到并移除UTF-8 BOM")
	}

	// 统一换行符：CRLF -> LF
	originalLen := len(b)
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	if len(b) != originalLen {
		log.Printf("🔧 [TEMPLATE] 已将CRLF转换为LF，字节数变化: %d -> %d", originalLen, len(b))
	}

	return b
}

// sha256Hex 计算SHA-256哈希并返回十六进制字符串
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LoadSystemTemplate 加载并规范化系统模板
// 🔥 关键功能：
// 1. 读取模板文件
// 2. 规范化字节（去BOM、统一换行符）
// 3. 计算SHA-256指纹
// 4. 打印模板元信息（启动时调用一次）
func LoadSystemTemplate(path string) (*TemplateBundle, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}

	// 规范化字节（确保跨平台一致性）
	norm := normalizeTemplateBytes(raw)

	// 计算SHA-256指纹
	hash := sha256Hex(norm)

	tb := &TemplateBundle{
		SystemTemplate: string(norm),
		HashSHA256:     hash,
		Bytes:          len(norm),
		SourcePath:     path,
	}

	// 启动时打印一次模板元信息
	log.Printf("📜 [TEMPLATE] 模板加载成功")
	log.Printf("   路径: %s", path)
	log.Printf("   SHA-256: %s", tb.HashSHA256)
	log.Printf("   字节数: %d bytes", tb.Bytes)
	log.Printf("   字符数: %d chars", len(tb.SystemTemplate))

	return tb, nil
}

// LogTemplateUse 每次使用模板前打印指纹（用于验证一致性）
// 🔥 目的：确认每次请求使用的模板 hash 一致，检测意外的模板变更
func (tb *TemplateBundle) LogTemplateUse(requestID string) {
	log.Printf("📜 [TEMPLATE_USE] req_id=%s sha256=%s bytes=%d",
		requestID, tb.HashSHA256, tb.Bytes)
}

// Verify 验证当前模板与磁盘文件一致性（可选，用于热更新检测）
func (tb *TemplateBundle) Verify() error {
	raw, err := os.ReadFile(tb.SourcePath)
	if err != nil {
		return fmt.Errorf("重新读取模板失败: %w", err)
	}

	norm := normalizeTemplateBytes(raw)
	currentHash := sha256Hex(norm)

	if currentHash != tb.HashSHA256 {
		return fmt.Errorf("模板指纹不匹配！加载时=%s，当前=%s（模板文件可能已被修改）",
			tb.HashSHA256, currentHash)
	}

	return nil
}
