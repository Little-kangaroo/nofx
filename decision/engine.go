package decision

import (
	"encoding/json"
	"fmt"
	"log"
	"nofx/market"
	"nofx/mcp"
	"nofx/pool"
	"strconv"
	"strings"
	"time"
)

// PositionInfo 持仓信息
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"` // 持仓更新时间戳（毫秒）
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // 账户净值
	AvailableBalance float64 `json:"available_balance"` // 可用余额
	TotalPnL         float64 `json:"total_pnl"`         // 总盈亏
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // 总盈亏百分比
	MarginUsed       float64 `json:"margin_used"`       // 已用保证金
	MarginUsedPct    float64 `json:"margin_used_pct"`   // 保证金使用率
	PositionCount    int     `json:"position_count"`    // 持仓数量
}

// CandidateCoin 候选币种（来自币种池）
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // 来源: "ai500" 和/或 "oi_top"
}

// OITopData 持仓量增长Top数据（用于AI决策参考）
type OITopData struct {
	Rank              int     // OI Top排名
	OIDeltaPercent    float64 // 持仓量变化百分比（1小时）
	OIDeltaValue      float64 // 持仓量变化价值
	PriceDeltaPercent float64 // 价格变化百分比
	NetLong           float64 // 净多仓
	NetShort          float64 // 净空仓
}

// Context 交易上下文（传递给AI的完整信息）
type Context struct {
	CurrentTime     string                  `json:"current_time"`
	RuntimeMinutes  int                     `json:"runtime_minutes"`
	CallCount       int                     `json:"call_count"`
	Account         AccountInfo             `json:"account"`
	Positions       []PositionInfo          `json:"positions"`
	CandidateCoins  []CandidateCoin         `json:"candidate_coins"`
	MarketDataMap   map[string]*market.Data `json:"-"` // 不序列化，但内部使用
	OITopDataMap    map[string]*OITopData   `json:"-"` // OI Top数据映射
	Performance     interface{}             `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	BTCETHLeverage  int                     `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage int                     `json:"-"` // 山寨币杠杆倍数（从配置读取）
}

// Decision AI的交易决策
type Decision struct {
	Symbol          string  `json:"symbol"`
	Action          string  `json:"action"` // "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`
	Confidence      int     `json:"confidence,omitempty"` // 信心度 (0-100)
	RiskUSD         float64 `json:"risk_usd,omitempty"`   // 最大美元风险
	Reasoning       string  `json:"reasoning"`
}

// FullDecision AI的完整决策（包含思维链）
type FullDecision struct {
	SystemPrompt string     `json:"system_prompt"` // 系统提示词（发送给AI的系统prompt）
	UserPrompt   string     `json:"user_prompt"`   // 发送给AI的输入prompt
	CoTTrace     string     `json:"cot_trace"`     // 思维链分析（AI输出）
	Decisions    []Decision `json:"decisions"`     // 具体决策列表
	Timestamp    time.Time  `json:"timestamp"`
}

// GetFullDecision 获取AI的完整交易决策（批量分析所有币种和持仓）
func GetFullDecision(ctx *Context, mcpClient *mcp.Client) (*FullDecision, error) {
	return GetFullDecisionWithCustomPrompt(ctx, mcpClient, "", false, "")
}

// GetFullDecisionWithCustomPrompt 获取AI的完整交易决策（支持自定义prompt和模板选择）
func GetFullDecisionWithCustomPrompt(ctx *Context, mcpClient *mcp.Client, customPrompt string, overrideBase bool, templateName string) (*FullDecision, error) {
	// 1. 为所有币种获取市场数据
	if err := fetchMarketDataForContext(ctx); err != nil {
		return nil, fmt.Errorf("获取市场数据失败: %w", err)
	}

	// 2. 构建 System Prompt（固定规则）和 User Prompt（动态数据）
	systemPrompt := buildSystemPromptWithCustom(ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, customPrompt, overrideBase, templateName)
	userPrompt := buildUserPrompt(ctx)

	// 3. 调用AI API（使用 system + user prompt）
	aiResponse, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("调用AI API失败: %w", err)
	}

	// 4. 解析AI响应
	decision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, templateName)
	if err != nil {
		return decision, fmt.Errorf("解析AI响应失败: %w", err)
	}

	decision.Timestamp = time.Now()
	decision.SystemPrompt = systemPrompt // 保存系统prompt
	decision.UserPrompt = userPrompt     // 保存输入prompt
	return decision, nil
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	log.Printf("🔍 [DEBUG] fetchMarketDataForContext开始，候选币种数量: %d", len(ctx.CandidateCoins))
	ctx.MarketDataMap = make(map[string]*market.Data)
	ctx.OITopDataMap = make(map[string]*OITopData)
	log.Printf("🔍 [DEBUG] MarketDataMap已初始化")

	// 收集所有需要获取数据的币种
	symbolSet := make(map[string]bool)

	// 1. 优先获取持仓币种的数据（这是必须的）
	for _, pos := range ctx.Positions {
		symbolSet[pos.Symbol] = true
	}

	// 2. 候选币种数量根据账户状态动态调整
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}
		symbolSet[coin.Symbol] = true
	}

	// 并发获取市场数据
	// 持仓币种集合（用于判断是否跳过OI检查）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	log.Printf("🔍 [DEBUG] 开始获取%d个币种的市场数据", len(symbolSet))
	for symbol := range symbolSet {
		log.Printf("🔍 [DEBUG] 正在获取 %s 的市场数据...", symbol)
		data, err := market.Get(symbol)
		if err != nil {
			log.Printf("❌ [ERROR] 获取 %s 市场数据失败: %v", symbol, err)
			continue
		}
		log.Printf("✅ [DEBUG] 成功获取 %s 的市场数据，当前价格: %.4f", symbol, data.CurrentPrice)

		// ⚠️ 流动性过滤：持仓价值低于15M USD的币种不做（多空都不做）
		// 持仓价值 = 持仓量 × 当前价格
		// 但现有持仓必须保留（需要决策是否平仓）
		isExistingPosition := positionSymbols[symbol]
		if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
			// 计算持仓价值（USD）= 持仓量 × 当前价格
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000 // 转换为百万美元单位
			if oiValueInMillions < 15 {
				log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < 15M)，跳过此币种 [持仓量:%.0f × 价格:%.4f]",
					symbol, oiValueInMillions, data.OpenInterest.Latest, data.CurrentPrice)
				continue
			}
		}

		ctx.MarketDataMap[symbol] = data
	}

	// 加载OI Top数据（不影响主流程）
	oiPositions, err := pool.GetOITopPositions()
	if err == nil {
		for _, pos := range oiPositions {
			// 标准化符号匹配
			symbol := pos.Symbol
			ctx.OITopDataMap[symbol] = &OITopData{
				Rank:              pos.Rank,
				OIDeltaPercent:    pos.OIDeltaPercent,
				OIDeltaValue:      pos.OIDeltaValue,
				PriceDeltaPercent: pos.PriceDeltaPercent,
				NetLong:           pos.NetLong,
				NetShort:          pos.NetShort,
			}
		}
	}

	log.Printf("🔍 [DEBUG] fetchMarketDataForContext完成，最终MarketDataMap大小: %d", len(ctx.MarketDataMap))
	for symbol := range ctx.MarketDataMap {
		log.Printf("🔍 [DEBUG] MarketDataMap包含币种: %s", symbol)
	}
	return nil
}

// calculateMaxCandidates 根据账户状态计算需要分析的候选币种数量
func calculateMaxCandidates(ctx *Context) int {
	// 直接返回候选池的全部币种数量
	// 因为候选池已经在 auto_trader.go 中筛选过了
	// 固定分析前20个评分最高的币种（来自AI500）
	return len(ctx.CandidateCoins)
}

// buildSystemPromptWithCustom 构建包含自定义内容的 System Prompt
func buildSystemPromptWithCustom(accountEquity float64, btcEthLeverage, altcoinLeverage int, customPrompt string, overrideBase bool, templateName string) string {
	// 如果覆盖基础prompt且有自定义prompt，只使用自定义prompt
	if overrideBase && customPrompt != "" {
		return customPrompt
	}

	// 获取基础prompt（使用指定的模板）
	basePrompt := buildSystemPrompt(accountEquity, btcEthLeverage, altcoinLeverage, templateName)

	// 如果没有自定义prompt，直接返回基础prompt
	if customPrompt == "" {
		return basePrompt
	}

	// 添加自定义prompt部分到基础prompt
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n")
	sb.WriteString("# 📌 个性化交易策略\n\n")
	sb.WriteString(customPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("注意: 以上个性化策略是对基础规则的补充，不能违背基础风险控制原则。\n")

	return sb.String()
}

// buildSystemPrompt 构建 System Prompt（使用模板+动态部分）
func buildSystemPrompt(accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string) string {
	var sb strings.Builder

	// 1. 加载提示词模板（核心交易策略部分）
	if templateName == "" {
		templateName = "default" // 默认使用 default 模板
	}

	template, err := GetPromptTemplate(templateName)
	if err != nil {
		// 如果模板不存在，记录错误并使用 default
		log.Printf("⚠️  提示词模板 '%s' 不存在，使用 default: %v", templateName, err)
		template, err = GetPromptTemplate("default")
		if err != nil {
			// 如果连 default 都不存在，使用内置的简化版本
			log.Printf("❌ 无法加载任何提示词模板，使用内置简化版本")
			sb.WriteString("你是专业的加密货币交易AI。请根据市场数据做出交易决策。\n\n")
		} else {
			sb.WriteString(template.Content)
			sb.WriteString("\n\n")
		}
	} else {
		sb.WriteString(template.Content)
		sb.WriteString("\n\n")
	}

	// 2. 硬约束（风险控制）- 动态生成
	sb.WriteString("# 硬约束（风险控制）\n\n")
	sb.WriteString("1. 风险回报比: 必须 ≥ 1:3（冒1%风险，赚3%+收益）\n")
	sb.WriteString("2. 最多持仓: 3个币种（质量>数量）\n")
	sb.WriteString(fmt.Sprintf("3. 单币仓位: 山寨%.0f-%.0f U(%dx杠杆) | BTC/ETH %.0f-%.0f U(%dx杠杆)\n",
		accountEquity*0.8, accountEquity*1.5, altcoinLeverage, accountEquity*5, accountEquity*10, btcEthLeverage))
	sb.WriteString("4. 保证金: 总使用率 ≤ 90%\n\n")

	// 3. 输出格式 - 动态生成
	sb.WriteString("#输出格式\n\n")
	sb.WriteString("第一步: 思维链（纯文本）\n")
	sb.WriteString("简洁分析你的思考过程\n\n")
	sb.WriteString("第二步: JSON决策数组\n\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"confidence\": 85, \"risk_usd\": 300, \"reasoning\": \"下跌趋势+MACD死叉\"},\n", btcEthLeverage, accountEquity*5))
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\", \"reasoning\": \"止盈离场\"}\n")
	sb.WriteString("]\n```\n\n")
	sb.WriteString("字段说明:\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | hold | wait\n")
	sb.WriteString("- `confidence`: 0-100（开仓建议≥75）\n")
	sb.WriteString("- 开仓时必填: leverage, position_size_usd, stop_loss, take_profit, confidence, risk_usd, reasoning\n\n")

	return sb.String()
}

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("时间: %s | 周期: #%d | 运行: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// BTC 市场
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("BTC: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// 账户
	sb.WriteString(fmt.Sprintf("账户: 净值%.2f | 余额%.2f (%.1f%%) | 盈亏%+.2f%% | 保证金%.1f%% | 持仓%d个\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	// 持仓（完整市场数据）
	if len(ctx.Positions) > 0 {
		sb.WriteString("## 当前持仓\n")
		for i, pos := range ctx.Positions {
			// 计算持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMs := time.Now().UnixMilli() - pos.UpdateTime
				durationMin := durationMs / (1000 * 60) // 转换为分钟
				if durationMin < 60 {
					holdingDuration = fmt.Sprintf(" | 持仓时长%d分钟", durationMin)
				} else {
					durationHour := durationMin / 60
					durationMinRemainder := durationMin % 60
					holdingDuration = fmt.Sprintf(" | 持仓时长%d小时%d分钟", durationHour, durationMinRemainder)
				}
			}

			sb.WriteString(fmt.Sprintf("%d. %s %s | 入场价%.4f 当前价%.4f | 盈亏%+.2f%% | 杠杆%dx | 保证金%.0f | 强平价%.4f%s\n\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, pos.UnrealizedPnLPct,
				pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration))

			// 使用FormatAsCompactData输出精简市场数据
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.FormatAsCompactData(marketData))
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("当前持仓: 无\n\n")
	}

	// 候选币种（完整市场数据）
	sb.WriteString(fmt.Sprintf("## 候选币种 (%d个)\n\n", len(ctx.MarketDataMap)))
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		displayedCount++

		sourceTags := ""
		if len(coin.Sources) > 1 {
			sourceTags = " (AI500+OI_Top双重信号)"
		} else if len(coin.Sources) == 1 && coin.Sources[0] == "oi_top" {
			sourceTags = " (OI_Top持仓增长)"
		}

		// 使用FormatAsCompactData输出精简市场数据
		sb.WriteString(fmt.Sprintf("### %d. %s%s\n\n", displayedCount, coin.Symbol, sourceTags))
		sb.WriteString(market.FormatAsCompactData(marketData))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// 夏普比率（直接传值，不要复杂格式化）
	if ctx.Performance != nil {
		// 直接从interface{}中提取SharpeRatio
		type PerformanceData struct {
			SharpeRatio float64 `json:"sharpe_ratio"`
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", perfData.SharpeRatio))
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n")

	return sb.String()
}

// parseFullDecisionResponse 解析AI的完整决策响应
func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string) (*FullDecision, error) {
	// 1. 提取思维链
	cotTrace := extractCoTTrace(aiResponse)

	// 2. 提取JSON决策列表
	decisions, err := extractDecisionsWithContext(aiResponse, accountEquity, btcEthLeverage, altcoinLeverage, templateName)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("提取决策失败: %w", err)
	}

	// 3. 验证决策
	if err := validateDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage, templateName); err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: decisions,
		}, fmt.Errorf("决策验证失败: %w", err)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

// extractCoTTrace 提取思维链分析
func extractCoTTrace(response string) string {
	// 查找JSON数组的开始位置
	jsonStart := strings.Index(response, "[")

	if jsonStart > 0 {
		// 思维链是JSON数组之前的内容
		return strings.TrimSpace(response[:jsonStart])
	}

	// 如果找不到JSON，整个响应都是思维链
	return strings.TrimSpace(response)
}


// extractDecisionsWithContext 提取JSON决策列表（带账户上下文）
func extractDecisionsWithContext(response string, accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string) ([]Decision, error) {
	// 直接查找JSON数组 - 找第一个完整的JSON数组
	arrayStart := strings.Index(response, "[")
	if arrayStart == -1 {
		return nil, fmt.Errorf("无法找到JSON数组起始")
	}

	// 从 [ 开始，匹配括号找到对应的 ]
	arrayEnd := findMatchingBracket(response, arrayStart)
	var jsonContent string
	if arrayEnd == -1 {
		log.Printf("🔍 AI响应JSON不完整，尝试自动修复...")
		log.Printf("🔍 原始响应片段: %s", response[arrayStart:min(arrayStart+300, len(response))])
		
		// 尝试修复不完整的JSON
		jsonContent = tryFixIncompleteJSON(response[arrayStart:])
		if jsonContent == "" {
			log.Printf("❌ JSON自动修复失败")
			return nil, fmt.Errorf("无法找到JSON数组结束，且无法自动修复\nJSON片段: %s", response[arrayStart:min(arrayStart+200, len(response))])
		} else {
			log.Printf("✅ JSON自动修复成功: %s", jsonContent)
		}
	} else {
		jsonContent = strings.TrimSpace(response[arrayStart : arrayEnd+1])
		log.Printf("🔍 找到完整JSON: %s", jsonContent[:min(200, len(jsonContent))])
	}

	// 🔧 修复常见的JSON格式错误：缺少引号的字段值
	jsonContent = fixMissingQuotes(jsonContent)

	// 先检查JSON内容是否是有效的决策数组格式
	if !isValidDecisionArray(jsonContent) {
		return nil, fmt.Errorf("AI返回的JSON格式无效，不是决策数组格式\nJSON内容: %s", jsonContent)
	}

	// 🎯 智能解析器选择：根据模板名优先选择对应的解析器
	log.Printf("🔍 [调试] 检测到模板: %s，选择对应解析策略", templateName)
	
	if strings.Contains(strings.ToLower(templateName), "taro") {
		// taro模板优先使用taro解析器
		log.Printf("🎯 [调试] 使用taro模板，优先尝试taro格式解析器")
		
		taroDecisions, taroErr := parseTaroFormatDecisions(jsonContent)
		if taroErr == nil {
			log.Printf("🔍 [调试] taro格式解析成功，数量: %d", len(taroDecisions))
			for i, d := range taroDecisions {
				log.Printf("🔍 [调试] taro决策#%d: Symbol=%s, Action=%s, StopLoss=%.6f", 
					i+1, d.Symbol, d.Action, d.StopLoss)
			}
			return taroDecisions, nil
		}
		log.Printf("⚠️ [调试] taro格式解析失败: %v，尝试标准格式", taroErr)
	}

	// 尝试解析为标准Decision格式（增强版，支持taro字段名）
	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err == nil {
		// 调试日志：打印解析后的决策内容
		log.Printf("🔍 [调试] 标准格式解析成功，数量: %d", len(decisions))
		for i, d := range decisions {
			log.Printf("🔍 [调试] 决策#%d: Symbol=%s, Action=%s, StopLoss=%.6f, TakeProfit=%.6f", 
				i+1, d.Symbol, d.Action, d.StopLoss, d.TakeProfit)
		}
		
		// 🔧 增强处理：检查是否有taro格式的字段需要转换
		decisions = enhanceDecisionsWithTaroFields(jsonContent, decisions)
		
		return decisions, nil
	}
	log.Printf("⚠️ [调试] 标准格式解析失败，尝试其他格式")

	// 如果不是taro模板，或者taro解析失败，尝试taro格式（兜底）
	if !strings.Contains(strings.ToLower(templateName), "taro") {
		taroDecisions, taroErr := parseTaroFormatDecisions(jsonContent)
		if taroErr == nil {
			log.Printf("🔍 [调试] 兜底taro格式解析成功，数量: %d", len(taroDecisions))
			for i, d := range taroDecisions {
				log.Printf("🔍 [调试] 兜底taro决策#%d: Symbol=%s, Action=%s, StopLoss=%.6f", 
					i+1, d.Symbol, d.Action, d.StopLoss)
			}
			return taroDecisions, nil
		}
		log.Printf("⚠️ [调试] 兜底taro格式解析失败: %v", taroErr)
	}

	// 如果taro格式失败，尝试解析混合格式（AI可能返回标准格式但某些字段类型不匹配）
	mixedDecisions, mixedErr := parseMixedFormatDecisions(jsonContent, accountEquity)
	if mixedErr == nil {
		return mixedDecisions, nil
	}

	// 如��混合格式也失败，尝试解析AI返回的复杂格式
	return parseComplexAIDecisions(jsonContent, accountEquity)
}

// parseTaroFormatDecisions 解析taro格式决策（使用actions数组和\"stop\"字段）
func parseTaroFormatDecisions(jsonContent string) ([]Decision, error) {
	// 定义taro格式的决策结构
	var taroResponse struct {
		Analysis struct {
			Symbol   string `json:"symbol"`
			MtfView  interface{} `json:"mtf_view"`
			Consensus string `json:"consensus"`
			Notes    string `json:"notes"`
		} `json:"analysis"`
		Actions []struct {
			Type         string  `json:"type"`          // "open|hold|reduce|close|update_stop"
			Side         string  `json:"side"`          // "LONG|SHORT"
			Qty          interface{} `json:"qty"`       // "number or percent for reduce" - 可能是字符串或数字
			Entry        interface{} `json:"entry"`     // "if open" - 可能是字符串或数字
			Stop         interface{} `json:"stop"`      // "new stop if any" - 关键字段，可能是字符串或数字
			TakeProfitHint string `json:"take_profit_hint"` // "可选：分段 TP 参考价/规则"
			Reason       string  `json:"reason"`        // "简洁、与模板规则一一对应"
		} `json:"actions"`
	}

	// 解析taro格式
	if err := json.Unmarshal([]byte(jsonContent), &taroResponse); err != nil {
		return nil, fmt.Errorf("taro格式JSON解析失败: %w", err)
	}

	// 从analysis中获取symbol
	symbol := taroResponse.Analysis.Symbol
	if symbol == "" {
		// 如果analysis中没有symbol，尝试使用默认的BTCUSDT
		symbol = "BTCUSDT"
		log.Printf("⚠️ taro格式中未找到symbol，使用默认值: %s", symbol)
	}

	// 转换为标准Decision格式
	var decisions []Decision
	for _, action := range taroResponse.Actions {
		decision := Decision{
			Symbol:    symbol,
			Action:    convertTaroActionToStandard(action.Type),
			Reasoning: action.Reason,
		}

		// 处理stop字段 -> StopLoss字段（关键修复）
		if action.Stop != nil {
			var stopPrice float64
			switch v := action.Stop.(type) {
			case string:
				if v != "" && v != "new stop if any" { // 跳过模板占位符
					if parsed, err := strconv.ParseFloat(v, 64); err == nil {
						stopPrice = parsed
					}
				}
			case float64:
				stopPrice = v
			case int:
				stopPrice = float64(v)
			}
			if stopPrice > 0 {
				decision.StopLoss = stopPrice
				log.Printf("🔍 [调试] taro格式解析: stop='%v' -> StopLoss=%.6f", action.Stop, stopPrice)
			}
		}

		// 处理side字段来确定具体的动作
		if action.Side == "LONG" {
			if decision.Action == "open" {
				decision.Action = "open_long"
			} else if decision.Action == "close" {
				decision.Action = "close_long"
			}
		} else if action.Side == "SHORT" {
			if decision.Action == "open" {
				decision.Action = "open_short"
			} else if decision.Action == "close" {
				decision.Action = "close_short"
			}
		}

		// 处理qty字段
		if action.Qty != nil {
			var quantity float64
			switch v := action.Qty.(type) {
			case string:
				if v != "" && v != "number or percent for reduce" { // 跳过模板占位符
					if parsed, err := strconv.ParseFloat(v, 64); err == nil {
						quantity = parsed
					}
				}
			case float64:
				quantity = v
			case int:
				quantity = float64(v)
			}
			if quantity > 0 {
				// 这里可能需要根据上下文判断是数量还是USD金额
				// 暂时假设是USD金额
				decision.PositionSizeUSD = quantity
			}
		}

		// 处理entry字段
		if action.Entry != nil {
			switch v := action.Entry.(type) {
			case string:
				if v != "" && v != "if open" { // 跳过模板占位符
					if parsed, err := strconv.ParseFloat(v, 64); err == nil {
						// 可以用于验证或记录，但Decision结构中没有EntryPrice字段
						_ = parsed
					}
				}
			case float64, int:
				// 处理数字类型的entry价格
			}
		}

		// 只有有效的决策才添加到列表中
		if decision.Action != "" && decision.Action != "wait" {
			decisions = append(decisions, decision)
		}
	}

	log.Printf("🔍 [调试] taro格式解析完成，共解析出%d个有效决策", len(decisions))
	for i, d := range decisions {
		log.Printf("🔍 [调试] 决策#%d: Action=%s, Symbol=%s, StopLoss=%.6f", 
			i+1, d.Action, d.Symbol, d.StopLoss)
	}

	return decisions, nil
}

// convertTaroActionToStandard 转换taro动作名称为标准格式
func convertTaroActionToStandard(taroAction string) string {
	switch taroAction {
	case "open":
		return "open" // 需要结合side字段确定方向
	case "hold":
		return "hold"
	case "reduce":
		return "reduce"
	case "close":
		return "close" // 需要结合side字段确定方向
	case "update_stop":
		return "update_stop"
	default:
		return "wait" // 未知动作默认为wait
	}
}

// extractDecisions 提取JSON决策列表（兼容性保留）
func extractDecisions(response string) ([]Decision, error) {
	// 直接查找JSON数组 - 找第一个完整的JSON数组
	arrayStart := strings.Index(response, "[")
	if arrayStart == -1 {
		return nil, fmt.Errorf("无法找到JSON数组起始")
	}

	// 从 [ 开始，匹配括号找到对应的 ]
	arrayEnd := findMatchingBracket(response, arrayStart)
	if arrayEnd == -1 {
		return nil, fmt.Errorf("无法找到JSON数组结束")
	}

	jsonContent := strings.TrimSpace(response[arrayStart : arrayEnd+1])

	// 🔧 修复常见的JSON格式错误：缺少引号的字段值
	jsonContent = fixMissingQuotes(jsonContent)

	// 尝试解析为标准Decision格式
	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err == nil {
		return decisions, nil
	}

	// 如果标准格式解析失败，尝试解析AI返回的复杂格式
	// 注意：这是兼容性函数，使用默认账户净值
	return parseComplexAIDecisions(jsonContent, 100.0) // 使用100 USDT作为默认账户净值
}


// parseMixedFormatDecisions 解析混合格式决策（标准格式但某些字段类型不匹配）
func parseMixedFormatDecisions(jsonContent string, accountEquity float64) ([]Decision, error) {
	// 定义灵活的决策结构，允许take_profit既可以是数字也可以是数组
	var mixedDecisions []struct {
		Symbol          string      `json:"symbol"`
		Action          string      `json:"action"`
		Leverage        int         `json:"leverage,omitempty"`
		PositionSizeUSD float64     `json:"position_size_usd,omitempty"`
		StopLoss        float64     `json:"stop_loss,omitempty"`
		TakeProfit      interface{} `json:"take_profit,omitempty"` // 允许数字或数组
		Confidence      int         `json:"confidence,omitempty"`
		RiskUSD         float64     `json:"risk_usd,omitempty"`
		Reasoning       string      `json:"reasoning"`
	}

	// 解析混合格式
	if err := json.Unmarshal([]byte(jsonContent), &mixedDecisions); err != nil {
		return nil, fmt.Errorf("混合格式JSON解析失败: %w", err)
	}

	// 转换为标准Decision格式
	var decisions []Decision
	for _, mixed := range mixedDecisions {
		decision := Decision{
			Symbol:          mixed.Symbol,
			Action:          mixed.Action,
			Leverage:        mixed.Leverage,
			PositionSizeUSD: mixed.PositionSizeUSD,
			StopLoss:        mixed.StopLoss,
			Confidence:      mixed.Confidence,
			RiskUSD:         mixed.RiskUSD,
			Reasoning:       mixed.Reasoning,
		}

		// 处理take_profit字段的类型变换
		if mixed.TakeProfit != nil {
			switch tp := mixed.TakeProfit.(type) {
			case float64:
				// 单个数字
				decision.TakeProfit = tp
			case []interface{}:
				// 数组，取第一个
				if len(tp) > 0 {
					if firstTP, ok := tp[0].(float64); ok {
						decision.TakeProfit = firstTP
					}
				}
			case []float64:
				// float64数组，取第一个
				if len(tp) > 0 {
					decision.TakeProfit = tp[0]
				}
			}
		}

		decisions = append(decisions, decision)
	}

	return decisions, nil
}

// parseComplexAIDecisions 解析AI返回的复杂格式并转换为标准Decision
func parseComplexAIDecisions(jsonContent string, accountEquity float64) ([]Decision, error) {
	// 定义AI返回的复杂格式结构
	var complexDecisions []struct {
		Symbol     string `json:"symbol"`
		Open       bool   `json:"open"`
		Side       string `json:"side"`
		Playbook   string `json:"playbook"`
		Entry      struct {
			Type      string  `json:"type"`
			Price     float64 `json:"price"`
			Tolerance float64 `json:"tolerance"`
		} `json:"entry"`
		StopLoss   float64   `json:"stop_loss"`
		TakeProfit []float64 `json:"take_profit"` // 注意这是数组
		MinRR      float64   `json:"min_rr"`
		Confluence float64   `json:"confluence_score"`
		Confidence int       `json:"confidence"`
		Positioning struct {
			RiskPerTrade    float64 `json:"risk_per_trade"`
			LeverageHint    int     `json:"leverage_hint"`
			SizeSafeguard   string  `json:"size_safeguard"`
		} `json:"positioning"`
		Routing struct {
			PostOnly     bool   `json:"post_only"`
			TimeInForce  string `json:"time_in_force"`
		} `json:"routing"`
		Reason            string   `json:"reason"`
		InsufficientData  []string `json:"insufficient_data"`
	}

	// 解析复杂格式
	if err := json.Unmarshal([]byte(jsonContent), &complexDecisions); err != nil {
		return nil, fmt.Errorf("复杂格式JSON解析失败: %w\nJSON内容: %s", err, jsonContent)
	}

	// 转换为标准Decision格式
	var decisions []Decision
	for _, complex := range complexDecisions {
		decision := Decision{
			Symbol:     complex.Symbol,
			Confidence: complex.Confidence,
			Reasoning:  complex.Reason,
		}

		// 转换动作类型
		if !complex.Open {
			// 不开仓，判断为hold或wait
			if complex.Side == "hold" {
				decision.Action = "hold"
			} else {
				decision.Action = "wait"
			}
		} else {
			// 开仓
			if complex.Side == "long" {
				decision.Action = "open_long"
			} else if complex.Side == "short" {
				decision.Action = "open_short"
			} else {
				decision.Action = "wait"
			}
		}

		// 对于开仓决策，填充详细信息
		if decision.Action == "open_long" || decision.Action == "open_short" {
			decision.Leverage = complex.Positioning.LeverageHint
			if decision.Leverage <= 0 {
				decision.Leverage = 5 // 默认5倍杠杆
			}
			
			decision.StopLoss = complex.StopLoss
			
			// 取第一个止盈价格
			if len(complex.TakeProfit) > 0 {
				decision.TakeProfit = complex.TakeProfit[0]
			}

			// 根据风险计算仓位大小，同时应用风控限制
			if complex.Positioning.RiskPerTrade > 0 && complex.Entry.Price > 0 && complex.StopLoss > 0 {
				// 使用实际账户净值
				riskAmount := accountEquity * complex.Positioning.RiskPerTrade
				priceDistance := 0.0
				if decision.Action == "open_long" {
					priceDistance = (complex.Entry.Price - complex.StopLoss) / complex.Entry.Price
				} else {
					priceDistance = (complex.StopLoss - complex.Entry.Price) / complex.Entry.Price
				}
				if priceDistance > 0 {
					decision.PositionSizeUSD = riskAmount / priceDistance
					decision.RiskUSD = riskAmount
				}
			}

			// 应用风控限制：山寨币最多1.5倍账户净值，BTC/ETH最多10倍
			var maxPositionSize float64
			if decision.Symbol == "BTCUSDT" || decision.Symbol == "ETHUSDT" {
				maxPositionSize = accountEquity * 10.0 // BTC/ETH最多10倍
			} else {
				maxPositionSize = accountEquity * 1.5 // 山寨币最多1.5倍
			}

			// 如果计算出的仓位过大，或者没有计算出仓位，使用安全的默认值
			if decision.PositionSizeUSD <= 0 || decision.PositionSizeUSD > maxPositionSize {
				// 使用账户净值的80%作为基础仓位，确保不超过限制
				basePosition := accountEquity * 0.8
				if decision.Symbol == "BTCUSDT" || decision.Symbol == "ETHUSDT" {
					decision.PositionSizeUSD = minFloat(basePosition*5, maxPositionSize) // BTC/ETH用5倍基础仓位
				} else {
					decision.PositionSizeUSD = minFloat(basePosition, maxPositionSize) // 山寨币用1倍基础仓位
				}
				decision.RiskUSD = accountEquity * 0.02 // 风险控制在2%
			}
		}

		decisions = append(decisions, decision)
	}

	return decisions, nil
}

// tryFixIncompleteJSON 尝试修复不完整的JSON数组
func tryFixIncompleteJSON(jsonFragment string) string {
	jsonFragment = strings.TrimSpace(jsonFragment)
	
	// 如果不是以[开始，返回空
	if !strings.HasPrefix(jsonFragment, "[") {
		return ""
	}
	
	// 检查是否是��单的缺少]的情况
	openCount := strings.Count(jsonFragment, "[")
	closeCount := strings.Count(jsonFragment, "]")
	
	if openCount > closeCount {
		// 尝试添加缺失的]
		needed := openCount - closeCount
		for i := 0; i < needed; i++ {
			jsonFragment += "]"
		}
		
		// 验证修复后的JSON是否有效
		var test []interface{}
		if err := json.Unmarshal([]byte(jsonFragment), &test); err == nil {
			return jsonFragment
		}
	}
	
	// 尝试修复不完整的对象
	braceOpenCount := strings.Count(jsonFragment, "{")
	braceCloseCount := strings.Count(jsonFragment, "}")
	
	if braceOpenCount > braceCloseCount {
		// 添加缺失的}
		needed := braceOpenCount - braceCloseCount
		for i := 0; i < needed; i++ {
			jsonFragment += "}"
		}
		// 然后添加数组结束符
		if !strings.HasSuffix(jsonFragment, "]") {
			jsonFragment += "]"
		}
		
		// 验证修复后的JSON是否有效
		var test []interface{}
		if err := json.Unmarshal([]byte(jsonFragment), &test); err == nil {
			return jsonFragment
		}
	}
	
	// 尝试查找最后一个完整的对象
	lastBrace := strings.LastIndex(jsonFragment, "}")
	if lastBrace == -1 {
		// 没有找到完整的对象，尝试其他方法
		// 查找最后一个逗号，截取到那里
		lastComma := strings.LastIndex(jsonFragment, ",")
		if lastComma > 0 {
			// 截取到最后一个逗号之前，然后尝试完成
			truncated := strings.TrimSpace(jsonFragment[:lastComma])
			if strings.Count(truncated, "{") > strings.Count(truncated, "}") {
				// 添加缺失的}
				needed := strings.Count(truncated, "{") - strings.Count(truncated, "}")
				for i := 0; i < needed; i++ {
					truncated += "}"
				}
			}
			truncated += "]"
			
			// 验证修���后的JSON是否有效
			var test []interface{}
			if err := json.Unmarshal([]byte(truncated), &test); err == nil {
				return truncated
			}
		}
		
		// 最后尝试：创建空数组
		log.Printf("⚠️ JSON修复失败，返回空数组。原始片段: %s", jsonFragment[:min(100, len(jsonFragment))])
		return "[]"
	}
	
	// 截取到最后一个完整对象，然后添加]
	fixedJSON := jsonFragment[:lastBrace+1] + "]"
	
	// 验证修复后的JSON是否有效
	var test []interface{}
	if err := json.Unmarshal([]byte(fixedJSON), &test); err == nil {
		return fixedJSON
	}
	
	// 如果所有修复尝试都失败，返回空数组以避免系统崩溃
	log.Printf("⚠️ JSON修复最终失败，返回空数组。原始片段: %s", jsonFragment[:min(100, len(jsonFragment))])
	return "[]"
}

// min 返回两个int中较小的值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// minFloat 返回两个float64中较小的值
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// isValidDecisionArray 检查JSON是否是有效的决策数组格式
func isValidDecisionArray(jsonContent string) bool {
	// 去除首尾空格
	jsonContent = strings.TrimSpace(jsonContent)
	
	// 必须以[]括起来
	if !strings.HasPrefix(jsonContent, "[") || !strings.HasSuffix(jsonContent, "]") {
		return false
	}
	
	// 检查是否为空数组
	if jsonContent == "[]" {
		return true
	}
	
	// 检查是否是纯数字数组（如[3292.86,3624.165]）
	var numbers []float64
	if err := json.Unmarshal([]byte(jsonContent), &numbers); err == nil {
		// 这是一个数字数组，不是决策数组
		log.Printf("⚠️ AI返回了数字数组而非决策数组")
		return false
	}
	
	// 检查是否包含决策对象的基本字段
	// 至少应该包含 "symbol" 字段
	if !strings.Contains(jsonContent, `"symbol"`) && !strings.Contains(jsonContent, `symbol`) {
		log.Printf("⚠️ AI返回的JSON不包含symbol字段")
		return false
	}
	
	// 检查是否是持仓数据而不是决策数据
	// 持仓数据通常包含: "side", "entry", "pnl_pct", "liq_price" 等字段
	// 决策数据应该包含: "action", "leverage", "position_size_usd" 等字段
	hasPositionFields := strings.Contains(jsonContent, `"side"`) && 
						strings.Contains(jsonContent, `"entry"`) && 
						strings.Contains(jsonContent, `"pnl_pct"`)
	
	hasDecisionFields := strings.Contains(jsonContent, `"action"`) || 
						strings.Contains(jsonContent, `"leverage"`) || 
						strings.Contains(jsonContent, `"position_size_usd"`)
	
	if hasPositionFields && !hasDecisionFields {
		log.Printf("⚠️ AI返回了持仓数据而非交易决策数据。包含字段: side, entry, pnl_pct")
		return false
	}
	
	return true
}

// fixMissingQuotes 替换中文引号为英文引号（避免输入法自动转换）
func fixMissingQuotes(jsonStr string) string {
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")  // '
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")  // '
	return jsonStr
}

// validateDecisions 验证所有决策（需要账户信息和杠杆配置）
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string) error {
	for i, decision := range decisions {
		if err := validateDecision(&decision, accountEquity, btcEthLeverage, altcoinLeverage, templateName); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}
	}
	return nil
}

// findMatchingBracket 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return -1
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// validateDecision 验证单个决策的有效性
func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string) error {
	// 验证action并标准化动作名称
	validActions := map[string]bool{
		"open_long":           true,
		"open_short":          true,
		"close_long":          true,
		"close_short":         true,
		"reduce":              true, // 减仓操作
		"reduce_long":         true, // 减多仓
		"reduce_short":        true, // 减空仓
		"update_stop":         true, // 更新止损（taro模板）
		"update_stop_loss":    true, // 更新止损（adaptive模板）
		"update_take_profit":  true, // 更新止盈
		"partial_close":       true, // 部分平仓
		"open":                true, // 通用开仓（需要结合side判断）
		"close":               true, // 通用平仓
		"hold":                true,
		"wait":                true,
		"buy_to_enter":        true, // 兼容提示词模板中的动作名
		"sell_to_enter":       true, // 兼容提示词模板中的动作名
		"buy":                 true, // 兼容简单的买入指令
		"sell":                true, // 兼容简单的卖出指令
	}

	// 标准化动作名称
	switch d.Action {
	case "buy_to_enter":
		d.Action = "open_long"
	case "sell_to_enter":
		d.Action = "open_short"
	case "buy":
		d.Action = "open_long"    // 默认将buy解释为开多
	case "sell":
		d.Action = "open_short"   // 默认将sell解释为开空
	case "reduce":
		// reduce需要根据当前持仓方向确定是reduce_long还是reduce_short
		// 这个逻辑在执行阶段处理，这里保持原样
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作必须提供完整参数
	if d.Action == "open_long" || d.Action == "open_short" {
		// 根据币种使用配置的杠杆上限
		maxLeverage := altcoinLeverage          // 山寨币使用配置的杠杆
		maxPositionValue := accountEquity * 1.5 // 山寨币最多1.5倍账户净值
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage          // BTC和ETH使用配置的杠杆
			maxPositionValue = accountEquity * 10 // BTC/ETH最多10倍账户净值
		}

		if d.Leverage <= 0 || d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆必须在1-%d之间（%s，当前配置上限%d倍）: %d", maxLeverage, d.Symbol, maxLeverage, d.Leverage)
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("仓位大小必须大于0: %.2f", d.PositionSizeUSD)
		}

		// 保证金验证移动到执行阶段（auto_trader.go），此处只���证逻辑合理性
		// 因为决策阶段的账户净值可能不是最新的可用余额
		// 验证仓位价值上限（加1%容差以避免浮点数精度问题）
		tolerance := maxPositionValue * 0.01 // 1%容差
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
				return fmt.Errorf("BTC/ETH单币种仓位价值不能超过%.0f USDT（10倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			} else {
				return fmt.Errorf("山寨币单币种仓位价值不能超过%.0f USDT（1.5倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			}
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}

		// 验证止损止盈的合理性和价格方向
		// 获取当前市价作为入场价参考
		marketData, err := market.Get(d.Symbol)
		var currentPrice float64 = 50000.0 // 默认价格，防止获取失败
		if err == nil {
			currentPrice = marketData.CurrentPrice
		}
		
		if d.Action == "open_long" {
			// 做多：止损 < 入场价 < 止盈
			if d.StopLoss >= currentPrice {
				return fmt.Errorf("做多时止损价(%.2f)必须低于当前价格(%.2f)", d.StopLoss, currentPrice)
			}
			if d.TakeProfit <= currentPrice {
				return fmt.Errorf("做多时止盈价(%.2f)必须高于当前价格(%.2f)", d.TakeProfit, currentPrice)
			}
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价(%.2f)必须小于止盈价(%.2f)", d.StopLoss, d.TakeProfit)
			}
		} else if d.Action == "open_short" {
			// 做空：止盈 < 入场价 < 止损
			if d.TakeProfit >= currentPrice {
				return fmt.Errorf("做空时止盈价(%.2f)必须低于当前价格(%.2f)", d.TakeProfit, currentPrice)
			}
			if d.StopLoss <= currentPrice {
				return fmt.Errorf("做空时止损价(%.2f)必须高于当前价格(%.2f)", d.StopLoss, currentPrice)
			}
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价(%.2f)必须大于止盈价(%.2f)", d.StopLoss, d.TakeProfit)
			}
		}

		// 验证风险回报比（必须≥1:3）
		// 使用当前市价作为入场价
		entryPrice := currentPrice

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			// 做多：风险 = (入场价 - 止损价) / 入场价
			//       收益 = (止盈价 - 入场价) / 入场价
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else if d.Action == "open_short" {
			// 做空：风险 = (止损价 - 入场价) / 入场价
			//       收益 = (入场价 - 止盈价) / 入场价
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		// 根据模板设置不同的风险回报比要求
		var minRiskRewardRatio float64
		if strings.Contains(strings.ToLower(templateName), "taro") {
			// taro模板：注重技术分析和动态管理，使用更宽松的标准
			minRiskRewardRatio = 2.0
		} else {
			// adaptive等其他模板：使用严格标准
			minRiskRewardRatio = 3.0
		}
		
		// 风险回报比不足时，不报错而是改为wait并说明原因
		if riskRewardRatio < minRiskRewardRatio {
			d.Action = "wait"
			d.Reasoning = fmt.Sprintf("风险回报比过低(%.2f:1)，最低要求%.1f:1，暂时观望 [风险:%.2f%% 收益:%.2f%%]",
				riskRewardRatio, minRiskRewardRatio, riskPercent, rewardPercent)
		}
	}

	// 验证update_stop和update_stop_loss动作必须提供止损价格
	if d.Action == "update_stop" || d.Action == "update_stop_loss" {
		if d.StopLoss <= 0 {
			return fmt.Errorf("update_stop动作必须提供有效的止损价格，当前为: %.6f", d.StopLoss)
		}
		
		// 获取当前市价用于合理性验证
		marketData, err := market.Get(d.Symbol)
		if err == nil {
			currentPrice := marketData.CurrentPrice
			// 基本的合理性检查：止损价格不应该偏离当前价格太远（50%以内）
			maxDeviation := currentPrice * 0.5
			if d.StopLoss > currentPrice+maxDeviation || d.StopLoss < currentPrice-maxDeviation {
				return fmt.Errorf("止损价格(%.2f)偏离当前价格(%.2f)过远，请检查", d.StopLoss, currentPrice)
			}
		}
	}

	// 验证update_take_profit动作必须提供止盈价格
	if d.Action == "update_take_profit" {
		if d.TakeProfit <= 0 {
			return fmt.Errorf("update_take_profit动作必须提供有效的止盈价格，当前为: %.6f", d.TakeProfit)
		}
		
		// 获取当前市价用于合理性验证
		marketData, err := market.Get(d.Symbol)
		if err == nil {
			currentPrice := marketData.CurrentPrice
			// 基本的合理性检查：止盈价格不应该偏离当前价格太远（100%以内）
			maxDeviation := currentPrice * 1.0
			if d.TakeProfit > currentPrice+maxDeviation || d.TakeProfit < currentPrice-maxDeviation {
				return fmt.Errorf("止盈价格(%.2f)偏离当前价格(%.2f)过远，请检查", d.TakeProfit, currentPrice)
			}
		}
	}

	return nil
}

// enhanceDecisionsWithTaroFields 增强决策解析，处理taro字段名（如stop字段）
func enhanceDecisionsWithTaroFields(jsonContent string, decisions []Decision) []Decision {
	// 解析原始JSON以获取taro格式字段
	var rawDecisions []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &rawDecisions); err != nil {
		log.Printf("⚠️ [调试] 无法解析JSON为通用格式，跳过taro字段增强: %v", err)
		return decisions
	}
	
	if len(rawDecisions) != len(decisions) {
		log.Printf("⚠️ [调试] 原始JSON和解析后决策数量不匹配，跳过增强")
		return decisions
	}
	
	log.Printf("🔧 [调试] 开始增强决策，检查taro字段...")
	
	for i := 0; i < len(decisions); i++ {
		rawDecision := rawDecisions[i]
		decision := &decisions[i]
		
		// 检查并处理stop字段 -> StopLoss
		if stopValue, exists := rawDecision["stop"]; exists && decision.StopLoss == 0 {
			var stopPrice float64
			switch v := stopValue.(type) {
			case string:
				if v != "" && v != "new stop if any" {
					if parsed, err := strconv.ParseFloat(v, 64); err == nil {
						stopPrice = parsed
					}
				}
			case float64:
				stopPrice = v
			case int:
				stopPrice = float64(v)
			}
			
			if stopPrice > 0 {
				decision.StopLoss = stopPrice
				log.Printf("🔧 [调试] 增强决策#%d: 发现stop字段=%.6f，设置StopLoss=%.6f", 
					i+1, stopValue, stopPrice)
			}
		}
		
		// 检查并处理take_profit字段的其他格式
		if tpValue, exists := rawDecision["take_profit"]; exists && decision.TakeProfit == 0 {
			var tpPrice float64
			switch v := tpValue.(type) {
			case string:
				if v != "" {
					if parsed, err := strconv.ParseFloat(v, 64); err == nil {
						tpPrice = parsed
					}
				}
			case float64:
				tpPrice = v
			case int:
				tpPrice = float64(v)
			}
			
			if tpPrice > 0 {
				decision.TakeProfit = tpPrice
				log.Printf("🔧 [调试] 增强决策#%d: 发现take_profit字段=%.6f", i+1, tpPrice)
			}
		}
	}
	
	log.Printf("🔧 [调试] 决策增强完成")
	return decisions
}
