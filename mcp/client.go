package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Provider AI提供商类型
type Provider string

const (
	ProviderDeepSeek Provider = "deepseek"
	ProviderQwen     Provider = "qwen"
	ProviderCustom   Provider = "custom"
)

// Client AI API配置
type Client struct {
	Provider      Provider
	APIKey        string
	BaseURL       string
	Model         string
	Timeout       time.Duration
	UseFullURL    bool              // 是否使用完整URL（不添加/chat/completions）
	CustomHeaders map[string]string // 自定义请求头
	RequestFormat string            // 请求格式：openai（默认）、anthropic
}

func New() *Client {
	// 默认配置
	return &Client{
		Provider:      ProviderDeepSeek,
		BaseURL:       "https://api.deepseek.com/v1",
		Model:         "deepseek-chat",
		Timeout:       600 * time.Second, // 增加到600秒（10分钟），减少超时发生
		CustomHeaders: make(map[string]string),
		RequestFormat: "openai", // 默认使用OpenAI格式
	}
}

// SetDeepSeekAPIKey 设置DeepSeek API密钥
// customURL 为空时使用默认URL，customModel 为空时使用默认模型
func (client *Client) SetDeepSeekAPIKey(apiKey string, customURL string, customModel string) {
	client.Provider = ProviderDeepSeek
	client.APIKey = apiKey
	if customURL != "" {
		client.BaseURL = customURL
		log.Printf("🔧 [MCP] DeepSeek 使用自定义 BaseURL: %s", customURL)
	} else {
		client.BaseURL = "https://api.deepseek.com/v1"
		log.Printf("🔧 [MCP] DeepSeek 使用默认 BaseURL: %s", client.BaseURL)
	}
	if customModel != "" {
		client.Model = customModel
		log.Printf("🔧 [MCP] DeepSeek 使用自定义 Model: %s", customModel)
	} else {
		client.Model = "deepseek-chat"
		log.Printf("🔧 [MCP] DeepSeek 使用默认 Model: %s", client.Model)
	}
	// 打印 API Key 的前后各4位用于验证
	if len(apiKey) > 8 {
		log.Printf("🔧 [MCP] DeepSeek API Key: %s...%s", apiKey[:4], apiKey[len(apiKey)-4:])
	}
}

// SetQwenAPIKey 设置阿里云Qwen API密钥
// customURL 为空时使用默认URL，customModel 为空时使用默认模型
func (client *Client) SetQwenAPIKey(apiKey string, customURL string, customModel string) {
	client.Provider = ProviderQwen
	client.APIKey = apiKey
	if customURL != "" {
		client.BaseURL = customURL
		log.Printf("🔧 [MCP] Qwen 使用自定义 BaseURL: %s", customURL)
	} else {
		client.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		log.Printf("🔧 [MCP] Qwen 使用默认 BaseURL: %s", client.BaseURL)
	}
	if customModel != "" {
		client.Model = customModel
		log.Printf("🔧 [MCP] Qwen 使用自定义 Model: %s", customModel)
	} else {
		client.Model = "qwen-plus" // 可选: qwen-turbo, qwen-plus, qwen-max
		log.Printf("🔧 [MCP] Qwen 使用默认 Model: %s", client.Model)
	}
	// 打印 API Key 的前后各4位用于验证
	if len(apiKey) > 8 {
		log.Printf("🔧 [MCP] Qwen API Key: %s...%s", apiKey[:4], apiKey[len(apiKey)-4:])
	}
}

// SetCustomAPI 设置自定义OpenAI兼容API
func (client *Client) SetCustomAPI(apiURL, apiKey, modelName string) {
	client.Provider = ProviderCustom
	client.APIKey = apiKey
	client.CustomHeaders = make(map[string]string) // 重置自定义请求头
	client.RequestFormat = "openai"                // 默认OpenAI格式

	// 检测是否是Anthropic API
	if strings.Contains(apiURL, "anthropic") || strings.Contains(apiURL, "claude") {
		client.RequestFormat = "anthropic"
		client.UseFullURL = true // Anthropic API使用完整URL
		if strings.HasSuffix(apiURL, "#") {
			client.BaseURL = strings.TrimSuffix(apiURL, "#")
		} else {
			client.BaseURL = apiURL
		}

		// 设置Anthropic专用请求头
		client.CustomHeaders["x-api-key"] = apiKey
		client.CustomHeaders["anthropic-version"] = "2023-06-01"
		client.CustomHeaders["content-type"] = "application/json"

		log.Printf("🔧 [MCP] 检测到Anthropic API，使用专用配置")
		log.Printf("🔧 [MCP] Anthropic BaseURL: %s", client.BaseURL)
		log.Printf("🔧 [MCP] 已设置Anthropic专用请求头")
	} else {
		// OpenAI兼容API的原有逻辑
		if strings.HasSuffix(apiURL, "#") {
			client.BaseURL = strings.TrimSuffix(apiURL, "#")
			client.UseFullURL = true
		} else {
			client.BaseURL = apiURL
			client.UseFullURL = false
		}
	}

	client.Model = modelName
	client.Timeout = 600 * time.Second // 增加到600秒，适应大模型长响应
}

// SetCustomHeaders 设置自定义请求头（高级功能）
func (client *Client) SetCustomHeaders(headers map[string]string) {
	if client.CustomHeaders == nil {
		client.CustomHeaders = make(map[string]string)
	}
	for key, value := range headers {
		client.CustomHeaders[key] = value
	}
	log.Printf("🔧 [MCP] 已设置%d个自定义请求头", len(headers))
}

// SetAnthropicAPI 专用方法：设置Anthropic Claude API
func (client *Client) SetAnthropicAPI(apiURL, apiKey, modelName string) {
	client.Provider = ProviderCustom
	client.APIKey = apiKey
	client.BaseURL = apiURL
	client.Model = modelName
	client.UseFullURL = true
	client.RequestFormat = "anthropic"
	client.Timeout = 600 * time.Second

	// 初始化自定义请求头
	client.CustomHeaders = map[string]string{
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
		"content-type":      "application/json",
	}

	log.Printf("🔧 [MCP] Anthropic API配置完成")
	log.Printf("🔧 [MCP] BaseURL: %s", client.BaseURL)
	log.Printf("🔧 [MCP] Model: %s", client.Model)
	log.Printf("🔧 [MCP] API Key: %s...%s", apiKey[:4], apiKey[len(apiKey)-4:])
}

// SetClient 设置完整的AI配置（高级用户）
func (client *Client) SetClient(Client Client) {
	if Client.Timeout == 0 {
		Client.Timeout = 600 * time.Second // 默认600秒超时
	}
	client = &Client
}

// CallWithMessages 使用 system + user prompt 调用AI API（推荐）
func (client *Client) CallWithMessages(systemPrompt, userPrompt string) (string, error) {
	if client.APIKey == "" {
		return "", fmt.Errorf("AI API密钥未设置，请先调用 SetDeepSeekAPIKey() 或 SetQwenAPIKey()")
	}

	// 重试配置
	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			// 检查是否是HTTP/2相关错误
			if lastErr != nil && strings.Contains(lastErr.Error(), "http2:") {
				fmt.Printf("⚠️  检测到HTTP/2问题，重试 (%d/%d) 使用优化配置...\n", attempt, maxRetries)
			} else {
				fmt.Printf("⚠️  AI API调用失败，正在重试 (%d/%d)...\n", attempt, maxRetries)
			}
		}

		result, err := client.callOnce(systemPrompt, userPrompt)
		if err == nil {
			if attempt > 1 {
				fmt.Printf("✓ AI API重试成功\n")
			}
			return result, nil
		}

		lastErr = err
		// 如果不是网络错误，不重试
		if !isRetryableError(err) {
			return "", err
		}

		// 重试前等待
		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * 2 * time.Second
			fmt.Printf("⏳ 等待%v后重试...\n", waitTime)
			time.Sleep(waitTime)
		}
	}

	return "", fmt.Errorf("重试%d次后仍然失败: %w", maxRetries, lastErr)
}

// callOnce 单次调用AI API（内部使用）
func (client *Client) callOnce(systemPrompt, userPrompt string) (string, error) {
	// 打印当前 AI 配置
	log.Printf("📡 [MCP] AI 请求配置:")
	log.Printf("   Provider: %s", client.Provider)
	log.Printf("   BaseURL: %s", client.BaseURL)
	log.Printf("   Model: %s", client.Model)
	log.Printf("   RequestFormat: %s", client.RequestFormat)
	if len(client.APIKey) > 8 {
		log.Printf("   API Key: %s...%s", client.APIKey[:4], client.APIKey[len(client.APIKey)-4:])
	}

	// 根据API格式构建请求体
	var requestBody map[string]interface{}
	var maxTokens int
	var messages []map[string]string // 🆕 在函数级别定义messages变量

	if client.RequestFormat == "anthropic" {
		// Anthropic Claude API 格式
		maxTokens = 4096 // Anthropic Claude默认token限制

		requestBody = map[string]interface{}{
			"model":      client.Model,
			"max_tokens": maxTokens,
		}

		// Anthropic API格���：system作为独立参数，messages只包含用户消息
		if systemPrompt != "" {
			requestBody["system"] = systemPrompt
		}

		// messages数组只包含用户消息
		messages = []map[string]string{
			{
				"role":    "user",
				"content": userPrompt,
			},
		}
		requestBody["messages"] = messages

		log.Printf("📤 [MCP] 使用Anthropic API格式")
	} else {
		// OpenAI兼容API格式（默认）
		switch client.Provider {
		case ProviderDeepSeek:
			maxTokens = 8192 // DeepSeek API 限制为 8192
		case ProviderQwen:
			maxTokens = 32768 // Qwen 支持更高的 token 限制
		case ProviderCustom:
			maxTokens = 8000 // 自定义 API 默认使用较高限制
		default:
			maxTokens = 8000 // 默认使用较保守的限制
		}

		// 构建 messages 数组

		// 如果有 system prompt，添加 system message
		if systemPrompt != "" {
			messages = append(messages, map[string]string{
				"role":    "system",
				"content": systemPrompt,
			})
		}

		// 添加 user message
		messages = append(messages, map[string]string{
			"role":    "user",
			"content": userPrompt,
		})

		// 构建请求体 - 支持新旧API格式，兼容ChatGPT-5和GPT-5.1
		requestBody = map[string]interface{}{
			"model":    client.Model,
			"messages": messages,
			// 移除temperature等采样参数以兼容ChatGPT-5和新版API
			// 让模型使用默认参数以获得最佳性能
		}

		// 根据不同的API提供商使用不同的token限制参数名
		// 新版OpenAI API要求使用max_completion_tokens而不是max_tokens
		switch client.Provider {
		case ProviderDeepSeek:
			requestBody["max_tokens"] = maxTokens // DeepSeek仍使用max_tokens
			requestBody["temperature"] = 0.5      // DeepSeek支持temperature参数
			// 🔧 新增：DeepSeek支持JSON格式输出
			requestBody["response_format"] = map[string]interface{}{
				"type": "json_object",
			}
		case ProviderQwen:
			requestBody["max_tokens"] = maxTokens // Qwen仍使用max_tokens
			requestBody["temperature"] = 0.5      // Qwen支持temperature参数
		case ProviderCustom:
			// 自定义API（通常是OpenAI兼容）- 支持GPT-5.1参数
			requestBody["max_completion_tokens"] = maxTokens

			// GPT-5.1专用参数
			if client.Model == "gpt-5.1" || strings.Contains(client.Model, "gpt-5") {
				requestBody["reasoning_effort"] = "low"
				requestBody["prompt_cache_retention"] = "24h"
			}
		default:
			// 默认使用新格式，支持GPT-5.1参数
			requestBody["max_completion_tokens"] = maxTokens

			// GPT-5.1专用参数
			if client.Model == "gpt-5.1" || strings.Contains(client.Model, "gpt-5") {
				requestBody["reasoning_effort"] = "low"
				requestBody["prompt_cache_retention"] = "24h"
			}
		}

		log.Printf("📤 [MCP] 使用OpenAI兼容API格式")
	}

	// 打印请求参数（脱敏）
	log.Printf("📤 [MCP] AI请求参数:")
	log.Printf("   Provider: %s", client.Provider)
	log.Printf("   Model: %s", client.Model)

	// 显示实际使用的参数
	if temp, hasTemp := requestBody["temperature"]; hasTemp {
		log.Printf("   Temperature: %v", temp)
	} else {
		log.Printf("   Temperature: 默认值 (兼容ChatGPT-5)")
	}

	// 显示实际使用的token参数名
	if _, hasMaxTokens := requestBody["max_tokens"]; hasMaxTokens {
		log.Printf("   Max Tokens (max_tokens): %d", maxTokens)
	} else {
		log.Printf("   Max Completion Tokens (max_completion_tokens): %d", maxTokens)
	}

	// 显示JSON格式输出设置
	if responseFormat, hasResponseFormat := requestBody["response_format"]; hasResponseFormat {
		if rf, ok := responseFormat.(map[string]interface{}); ok {
			if rfType, ok := rf["type"].(string); ok {
				log.Printf("   Response Format: %s (强制JSON输出)", rfType)
			}
		}
	}

	// 显示GPT-5.1专用参数
	if reasoningEffort, hasReasoning := requestBody["reasoning_effort"]; hasReasoning {
		log.Printf("   Reasoning Effort: %v", reasoningEffort)
	}
	if cacheRetention, hasCache := requestBody["prompt_cache_retention"]; hasCache {
		log.Printf("   Prompt Cache Retention: %v", cacheRetention)
	}

	log.Printf("   Messages Count: %d", len(messages))
	if systemPrompt != "" {
		log.Printf("   System Prompt Length: %d chars", len(systemPrompt))
	}
	log.Printf("   User Prompt Length: %d chars", len(userPrompt))

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}
	log.Printf("📤 [MCP] JSON请求体大小: %d bytes", len(jsonData))

	// 创建HTTP请求
	var url string
	if client.UseFullURL {
		// 使用完整URL，不添加/chat/completions
		url = client.BaseURL
	} else {
		// 默认行为：添加/chat/completions
		url = fmt.Sprintf("%s/chat/completions", client.BaseURL)
	}
	log.Printf("📡 [MCP] 请求 URL: %s", url)
	log.Printf("📡 [MCP] 客户端超时设置: %v", client.Timeout)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// 根据不同的Provider设置认证方式
	switch client.Provider {
	case ProviderDeepSeek:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
	case ProviderQwen:
		// 阿里云Qwen使用API-Key认证
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
		// 注意：如果使用的不是兼容模式，可能需要不同的认证方式
	default:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
	}

	// 🆕 为Anthropic API设置自定义请求头
	if client.RequestFormat == "anthropic" {
		// 使用自定义请求头而不是Authorization header
		for headerKey, headerValue := range client.CustomHeaders {
			req.Header.Set(headerKey, headerValue)
		}
		log.Printf("🔧 [MCP] 已设置Anthropic专用请求头: %d个", len(client.CustomHeaders))
	} else {
		// 根据不同的Provider设置认证方式（OpenAI兼容API）
		switch client.Provider {
		case ProviderDeepSeek:
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
		case ProviderQwen:
			// 阿里云Qwen使用API-Key认证
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
			// 注意：如果使用的不是兼容模式，可能需要不同的认证方式
		default:
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
		}
	}

	// 发送请求
	httpClient := &http.Client{
		Timeout: client.Timeout,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   30 * time.Second,  // TLS握手超时
			ResponseHeaderTimeout: 300 * time.Second, // 增加到300秒等待响应头
			ExpectContinueTimeout: 1 * time.Second,   // Expect: 100-continue 超时
			ForceAttemptHTTP2:     false,             // 禁用HTTP/2，强制使用HTTP/1.1
			MaxIdleConns:          10,                // 最大空闲连接
			IdleConnTimeout:       30 * time.Second,  // 空闲连接超时
			DisableKeepAlives:     false,             // 启用keep-alive
		},
	}

	// 记录请求开始时间
	requestStart := time.Now()
	log.Printf("📡 [MCP] 开始发送AI请求: %v", requestStart.Format("15:04:05"))
	resp, err := httpClient.Do(req)
	requestDuration := time.Since(requestStart)
	log.Printf("📊 [AI请求耗时] HTTP请求耗时: %v", requestDuration)

	if err != nil {
		log.Printf("❌ [MCP] 请求失败，耗时: %v, 错误: %v", requestDuration, err)
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("✅ [MCP] 请求成功，耗时: %v, 状态码: %d, 协议: %s", requestDuration, resp.StatusCode, resp.Proto)

	// 读取和处理响应阶段耗时统计
	responseProcessStart := time.Now()
	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误 (status %d): %s", resp.StatusCode, string(body))
	}

	// 🆕 根据API格式解析响应
	var responseContent string
	if client.RequestFormat == "anthropic" {
		// Anthropic Claude API 响应格式
		var anthropicResult struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}

		if err := json.Unmarshal(body, &anthropicResult); err != nil {
			return "", fmt.Errorf("解析Anthropic响应失败: %w", err)
		}

		if len(anthropicResult.Content) == 0 {
			return "", fmt.Errorf("Anthropic API返回空内容")
		}

		// 拼接所有文本内容
		for _, content := range anthropicResult.Content {
			if content.Type == "text" {
				responseContent += content.Text
			}
		}

		if responseContent == "" {
			return "", fmt.Errorf("Anthropic API未返回文本内容")
		}

		log.Printf("📥 [MCP] Anthropic响应解析成功: %d个内容块", len(anthropicResult.Content))
	} else {
		// OpenAI兼容API响应格式（默认）
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return "", fmt.Errorf("解析OpenAI响应失败: %w", err)
		}

		if len(result.Choices) == 0 {
			return "", fmt.Errorf("OpenAI API返回空响应")
		}

		responseContent = result.Choices[0].Message.Content
		log.Printf("📥 [MCP] OpenAI响应解析成功: %d个choices", len(result.Choices))
	}

	// 响应处理耗时统计
	responseProcessDuration := time.Since(responseProcessStart)
	totalRequestDuration := time.Since(requestStart)

	// 写入单纯的请求和响应体到文件
	writeSimpleAPILog(jsonData, responseContent, client)

	// AI请求完整耗时统计
	log.Printf("📊 [AI请求耗时统计] 总耗时: %v | HTTP请求: %v (%.1f%%) | 响应处理: %v (%.1f%%)",
		totalRequestDuration,
		requestDuration, float64(requestDuration.Nanoseconds())/float64(totalRequestDuration.Nanoseconds())*100,
		responseProcessDuration, float64(responseProcessDuration.Nanoseconds())/float64(totalRequestDuration.Nanoseconds())*100)

	// 记录响应信息和潜在的截断警告
	log.Printf("📥 [MCP] AI响应接收: %d 字符", len(responseContent))
	if len(responseContent) >= 30000 { // 接近32K字符限制
		log.Printf("⚠️ [MCP] 响应长度接近token限制，检查是否被截断")
	}

	// 检查响应的结束是否自然
	trimmedResponse := strings.TrimSpace(responseContent)
	if len(trimmedResponse) > 0 {
		// 检查响应是否以自然的结束符结尾
		endsNaturally := strings.HasSuffix(trimmedResponse, ".") ||
			strings.HasSuffix(trimmedResponse, "。") ||
			strings.HasSuffix(trimmedResponse, "}") ||
			strings.HasSuffix(trimmedResponse, "]") ||
			strings.HasSuffix(trimmedResponse, "\n")

		if !endsNaturally {
			// 获取最后几个字符用于调试显示
			start := len(trimmedResponse) - 10
			if start < 0 {
				start = 0
			}
			lastChars := trimmedResponse[start:]
			log.Printf("⚠️ [MCP] 响应结尾可能被截断，最后%d字符: '%s'", len(lastChars), lastChars)
		}
	}

	return responseContent, nil
}

// isRetryableError 判断错误是否可重试（避免对超时错误重试以节省费用）
func isRetryableError(err error) bool {
	errStr := err.Error()
	// 超时错误不重试，避免额外费用
	timeoutErrors := []string{
		"timeout",
		"context deadline exceeded",                // 上下文超时
		"Client.Timeout exceeded",                  // 客户端超时
		"http2: timeout awaiting response headers", // HTTP/2响应头超时
	}
	for _, timeoutErr := range timeoutErrors {
		if strings.Contains(errStr, timeoutErr) {
			log.Printf("⚠️ [费用控制] 检测到超时错误，不重试以避免额外费用: %s", timeoutErr)
			return false // 超时错误不重试
		}
	}

	// 只对真正的网络连接问题重试
	retryableErrors := []string{
		"EOF",
		"connection reset",
		"connection refused",
		"temporary failure",
		"no such host",
	}
	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}
	return false
}

// writeSimpleAPILog 写入单纯的请求和响应体到文件
func writeSimpleAPILog(requestJSON []byte, responseContent string, client *Client) {
	// 获取模型信息
	provider := string(client.Provider)
	model := client.Model
	if model == "" {
		model = "unknown"
	}

	// 使用时间戳和模型信息创建文件名
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("ai_log_%s_%s_%s.txt", provider, model, timestamp)

	file, err := os.Create(filename)
	if err != nil {
		log.Printf("⚠️ 无法创建AI日志文件: %v", err)
		return
	}
	defer file.Close()

	// 写入请求体
	fmt.Fprintf(file, "=== REQUEST BODY ===\n")
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, requestJSON, "", "  "); err == nil {
		fmt.Fprintf(file, "%s\n", prettyJSON.String())
	} else {
		fmt.Fprintf(file, "%s\n", string(requestJSON))
	}

	// 写入响应体
	fmt.Fprintf(file, "\n=== RESPONSE BODY ===\n")
	fmt.Fprintf(file, "%s\n", responseContent)

	log.Printf("📝 AI请求响应已写入文件: %s", filename)
}
