package direction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// neutralOrderflow 构建一个"中性"订单流
// 通过所有 isOFStale 检查，但所有方向信号为零
// 用于隔离测试 structDir（channel 修复）对最终方向的影响
func neutralOrderflow() Orderflow {
	return Orderflow{
		Quality: Quality{
			Status:               "正常",
			OverallScore:         0.70,
			DataLagMs:            100,
			UpdateIntervalS:      5,
			CvdReliability:       0.70,
			OIReliability:        0.70,
			OrderbookReliability: 0.70,
		},
		Macro: Macro{
			TrendAlignment:  "divergent_mixed", // 宏观中性
			SignalStrength:  0,
			ConfidenceLevel: 0,
		},
		Micro5m: Micro5m{
			CandleIntent:    "mixed",
			FuturesCvdDelta: 0,
			SpotCvdDelta:    0,
			OIDeltaPct:      0,
			PriceDeltaPct:   0,
			VolumeDelta:     0,
			DataQuality:     0.70,
		},
		OB: Orderbook{
			ImbalanceRatio: 0,
			BidPressure:    0.50,
			AskPressure:    0.50,
			LiquidityScore: 0.50,
			SpoofingRisk:   0,
		},
	}
}

// parseChannelFromMap 使用修复后的字段名解析通道数据
func parseChannelFromMap(data map[string]interface{}) ChannelInfo {
	chData, ok := data["通道数据"].(map[string]interface{})
	if !ok {
		return ChannelInfo{Direction: "sideways", CurrentPosition: "inside", PriceRatio: 0.5, Quality: 0.5}
	}
	dirVal, _ := chData["channel_direction"].(string)
	posVal, _ := chData["current_position"].(string)
	ratioVal, _ := chData["price_ratio"].(float64)
	qualVal, _ := chData["quality"].(float64)
	if ratioVal == 0 {
		ratioVal = 0.5
	}
	if qualVal == 0 {
		qualVal = 0.5
	}
	return ChannelInfo{
		Direction:       dirVal,
		CurrentPosition: posVal,
		PriceRatio:      ratioVal,
		Quality:         qualVal,
	}
}

// parseVPVRFromMap 解析 VPVR 数据
func parseVPVRFromMap(data map[string]interface{}) VPVR {
	vpvrData, ok := data["VPVR数据"].(map[string]interface{})
	if !ok {
		return VPVR{}
	}
	vah, _ := vpvrData["dist_to_vah_atr"].(float64)
	val, _ := vpvrData["dist_to_val_atr"].(float64)
	return VPVR{DistToVAHATR: vah, DistToVALATR: val}
}

// extractRecordJSON 从 records 文本文件中提取 symbol→JSON 映射
func extractRecordJSON(content string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})
	// 匹配类似 "BTCUSDT: 65569.8000 ...{...}" 的行
	re := regexp.MustCompile(`([A-Z]+USDT):\s+[\d.]+[^\{]*(\{.+)`)
	for _, line := range strings.Split(content, "\n") {
		m := re.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		sym := m[1]
		jsonStr := m[2]
		var blob map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &blob); err != nil {
			continue
		}
		symData, ok := blob[sym].(map[string]interface{})
		if !ok {
			continue
		}
		result[sym] = symData
	}
	return result
}

// TestRecordsReplay 使用生产亏损记录数据，对比修复前后的方向决策
func TestRecordsReplay(t *testing.T) {
	recordsDir := "../../records"
	files, err := filepath.Glob(filepath.Join(recordsDir, "*.txt"))
	if err != nil || len(files) == 0 {
		t.Fatalf("未找到 records 目录或文件: %v", err)
	}

	cfg := DefaultConfig()

	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════")
	fmt.Println("  生产记录回放测试：修复后方向决策对比")
	fmt.Println("════════════════════════════════════════════════════════════════")

	for _, fpath := range files {
		fname := filepath.Base(fpath)
		content, err := os.ReadFile(fpath)
		if err != nil {
			t.Logf("读取失败: %s", fpath)
			continue
		}

		records := extractRecordJSON(string(content))
		if len(records) == 0 {
			t.Logf("未提取到数据: %s", fname)
			continue
		}

		for sym, data := range records {
			// ── 读取旧的方向决策 ──────────────────────────────────
			oldDir := "?"
			if da, ok := data["direction_arbitration"].(map[string]interface{}); ok {
				if ps, ok := da["plan_side"].(string); ok {
					oldDir = ps
				}
			}

			// ── 读取 AI 实际开仓方向（从 AI 决策块中）───────────
			aiAction := extractAIAction(string(content))

			// ── 解析 MTF 数据（用修复后的字段名）─────────────────
			mtfRaw, _ := data["多时间框架分析"].(map[string]interface{})
			mtf := make(MTFAnalysis)
			for _, tf := range []string{"30m", "15m"} {
				tfData, ok := mtfRaw[tf].(map[string]interface{})
				if !ok {
					continue
				}
				mtf[tf] = TimeframeData{
					Channel: parseChannelFromMap(tfData),
					VPVR:    parseVPVRFromMap(tfData),
				}
			}

			// ── 计算新的 structDir ────────────────────────────────
			newStructDir := StructDirFromMTF(mtf)

			// ── 运行完整 ComputeDirectionArbitration（中性订单流）─
			newResult := ComputeDirectionArbitration(RootSymbolInput{
				Symbol:    sym,
				Orderflow: neutralOrderflow(),
				MTF:       mtf,
			}, cfg)

			// ── 打印详情 ──────────────────────────────────────────
			fmt.Printf("\n【%s】 %s\n", fname, sym)
			fmt.Printf("  旧 direction_arbitration.plan_side = %s\n", oldDir)
			fmt.Printf("  AI 实际开仓方向                   = %s\n", aiAction)
			fmt.Println("  ─────────────────────────────────────────────────")

			// 逐时间框架展示通道信号变化
			for _, tf := range []string{"30m", "15m"} {
				if td, ok := mtf[tf]; ok {
					ch := td.Channel
					oldSign := legacyChannelSign(ch.Direction, ch.CurrentPosition)
					newSign := channelSign(ch.Direction, ch.CurrentPosition, ch.PriceRatio)
					fmt.Printf("  %s: dir=%-8s pos=%-12s │ 旧channelSign=%+.1f → 新channelSign=%+.1f\n",
						tf, ch.Direction, ch.CurrentPosition, oldSign, newSign)
				}
			}

			fmt.Printf("  ─────────────────────────────────────────────────\n")
			fmt.Printf("  新 struct_dir                     = %+.4f\n", newStructDir)
			fmt.Printf("  新 plan_side（中性订单流）         = %s\n", newResult.PlanSide)
			fmt.Printf("  新 delta / confidence             = %+.3f / %.3f\n", newResult.Delta, newResult.Confidence)
			fmt.Printf("  新 flags                          = %v\n", newResult.Flags)

			// ── 判断是否修正 ──────────────────────────────────────
			switch sym {
			case "DOGEUSDT":
				// DOGE：channel 数据为空（dir=""），structDir 仍接近 0
				// 根因是上游 channel 计算模块未产生方向，非 adapter 的 key bug
				fmt.Printf("  ⚠️  DOGE channel 数据本身为空（dir=\"\"），structDir 无法改善；\n")
				fmt.Printf("     根因在上游 channel 计算模块，不在 adapter key 修复范围内。\n")
			case "BTCUSDT":
				if newResult.PlanSide == SideShort || newResult.PlanSide == SideNeutral {
					fmt.Printf("  ✅ 修复有效：旧=%s → 新=%s（structDir 从~0 变为 %+.2f）\n",
						oldDir, newResult.PlanSide, newStructDir)
				} else {
					fmt.Printf("  ❌ 仍需关注：旧=%s → 新=%s\n", oldDir, newResult.PlanSide)
				}
			case "BNBUSDT":
				if oldDir == "SHORT" {
					fmt.Printf("  ✅ direction_arbitration 本身已正确输出 SHORT；\n")
					fmt.Printf("     根因是 AI COUNTERTREND 覆盖，已由 prompt 修复（Task4）。\n")
				}
			}

			t.Logf("[%s/%s] old=%s AI=%s → new=%s struct_dir=%+.4f",
				fname, sym, oldDir, aiAction, newResult.PlanSide, newStructDir)
		}
	}

	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════")
}

// legacyChannelSign 模拟修复前的 channelSign 行为（无下划线兼容，key 为 "通道分析数据" 时始终走默认）
// 这里直接模拟"旧代码走 default 分支"的效果
func legacyChannelSign(direction, position string) float64 {
	// 修复前：parseChannel 因 key 错误永远返回 "sideways"+"inside"
	// channelSign("sideways","inside",0.5) → dir=sideways → baseSign=0 → pos=inside → return 0*0.5=0
	_ = direction
	_ = position
	return 0.0
}

// extractAIAction 从记录文本中提取 AI 开仓方向
func extractAIAction(content string) string {
	// 找 "action":"open_long" 或 "action":"open_short"
	if strings.Contains(content, `"action":"open_long"`) {
		return "open_long (LONG)"
	}
	if strings.Contains(content, `"action":"open_short"`) {
		return "open_short (SHORT)"
	}
	return "unknown"
}
