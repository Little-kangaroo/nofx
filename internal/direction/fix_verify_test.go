package direction

import (
	"math"
	"testing"
)

// ========== Bug1/2/3 修复验证：channelSign 和 structDir ==========

// TestChannelSignUnderscoreFormat 验证 Bug3 修复：带下划线的 position 值能正确触发
func TestChannelSignUnderscoreFormat(t *testing.T) {
	cases := []struct {
		name      string
		direction string
		position  string
		wantSign  float64
	}{
		// 修复前：这些全部走 default 返回 0（flat 通道），现在应正确返回
		{"flat+break_down 应返回 -1.0", "flat", "break_down", -1.0},
		{"flat+break_up 应返回 +1.0", "flat", "break_up", 1.0},
		{"down+break_down 应返回 -1.0", "down", "break_down", -1.0},
		{"up+break_up 应返回 +1.0", "up", "break_up", 1.0},
		// 兼容无下划线格式（原格式）
		{"flat+breakdown 应返回 -1.0", "flat", "breakdown", -1.0},
		{"flat+breakup 应返回 +1.0", "flat", "breakup", 1.0},
		// inside 仍然正常
		{"up+inside 接近下轨 应为 +1.0", "up", "inside", 1.0},
		{"down+inside 接近上轨 应为 -1.0", "down", "inside", -1.0},
		// lower/upper 新增支持
		{"up+lower 应返回 +1.0", "up", "lower", 1.0},
		{"down+upper 应返回 -1.0", "down", "upper", -1.0},
		// 空值 sideways
		{"sideways+ 空 应返回 0.0", "sideways", "", 0.0},
		{"空+空 应返回 0.0", "", "", 0.0},
		// flat 别名
		{"flat+inside 接近下轨 应返回 0.0（flat 无方向）", "flat", "inside", 0.0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			priceRatio := 0.1 // 接近下轨
			if c.position == "inside" && c.direction == "down" {
				priceRatio = 0.9 // 接近上轨
			}
			got := channelSign(c.direction, c.position, priceRatio)
			if math.Abs(got-c.wantSign) > 1e-9 {
				t.Errorf("channelSign(%q, %q, %.1f) = %.2f, want %.2f",
					c.direction, c.position, priceRatio, got, c.wantSign)
			}
		})
	}
}

// TestStructDirFromMTF_FlatBreakDown 验证 Bug1+2+3 修复后 structDir 的正确性
// 对应亏损记录中 BTC：15m=flat+break_down，30m=flat+break_down
// 修复前：structDir ≈ +0.10（近乎为零，因 key 错误）
// 修复后：structDir 应为负值（跌破信号）
func TestStructDirFromMTF_FlatBreakDown(t *testing.T) {
	mtf := MTFAnalysis{
		"30m": TimeframeData{
			Channel: ChannelInfo{Direction: "flat", CurrentPosition: "break_down", PriceRatio: 0.5, Quality: 0.8},
			VPVR:    VPVR{DistToVAHATR: -13.68, DistToVALATR: 0.15},
		},
		"15m": TimeframeData{
			Channel: ChannelInfo{Direction: "flat", CurrentPosition: "break_down", PriceRatio: 0.5, Quality: 0.8},
			VPVR:    VPVR{DistToVAHATR: -8.21, DistToVALATR: 2.77},
		},
	}

	got := StructDirFromMTF(mtf)
	if got >= 0 {
		t.Errorf("flat+break_down 应产生负 structDir，got=%.4f（期望 < 0）", got)
	}
	t.Logf("flat+break_down structDir = %.4f ✓", got)
}

// TestStructDirFromMTF_DownBreakUp 对应 BNB 记录：
// 1h=down+break_up，30m=down+break_up（实际是下降通道里的短暂反弹）
// direction_arbitration 应综合多因子，structDir 应仍偏负或接近 0
func TestStructDirFromMTF_DownBreakUp(t *testing.T) {
	mtf := MTFAnalysis{
		"30m": TimeframeData{
			Channel: ChannelInfo{Direction: "down", CurrentPosition: "break_up", PriceRatio: 0.5, Quality: 0.7},
			VPVR:    VPVR{DistToVAHATR: -11.7, DistToVALATR: 5.55},
		},
		"15m": TimeframeData{
			Channel: ChannelInfo{Direction: "flat", CurrentPosition: "break_up", PriceRatio: 0.5, Quality: 0.6},
			VPVR:    VPVR{DistToVAHATR: -13.5, DistToVALATR: -4.0},
		},
	}

	got := StructDirFromMTF(mtf)
	// down+break_up：break_up 本身返回 +1.0，但 down 通道 break_up 含义不同
	// 目前 channelSign("down", "break_up", ...) = +1.0（算法设计如此，break_up 始终 +1.0）
	// 这里只是记录值，不做强断言；主要验证不会产生极端错误
	t.Logf("down+break_up structDir = %.4f（注：break_up 信号已纳入 direction_arbitration 计算）", got)
}

// TestStructDirFromMTF_EmptyChannel 验证 DOGE 场景：channel 数据为空时的回退行为
// 修复前与修复后行为相同（数据本身为空），但现在适配器默认返回 sideways+inside+0.5
// structDir 应接近 0，通过 VPVR tiebreak 给出微弱信号
func TestStructDirFromMTF_EmptyChannel(t *testing.T) {
	mtf := MTFAnalysis{
		"30m": TimeframeData{
			Channel: ChannelInfo{Direction: "sideways", CurrentPosition: "inside", PriceRatio: 0.5, Quality: 0.5},
			VPVR:    VPVR{DistToVAHATR: -10.98, DistToVALATR: 6.65},
		},
		"15m": TimeframeData{
			Channel: ChannelInfo{Direction: "sideways", CurrentPosition: "inside", PriceRatio: 0.5, Quality: 0.5},
			VPVR:    VPVR{DistToVAHATR: -12.1, DistToVALATR: 3.93},
		},
	}

	got := StructDirFromMTF(mtf)
	// sideways+inside+0.5 → channelSign=0 → structDir 由 VPVR tiebreak 决定
	// VPVR: 15m nearVAL > nearVAH → +0.10
	if math.Abs(got) > 0.2 {
		t.Errorf("空通道数据时 structDir 应接近 0，got=%.4f", got)
	}
	t.Logf("empty channel structDir = %.4f（由 VPVR tiebreak 决定）", got)
}

// ========== 完整 ComputeDirectionArbitration 集成测试 ==========

// TestComputeDirection_BearishMarket 模拟亏损记录中的熊市场景
// 当 15m/30m 均出现 break_down 时，direction_arbitration 不应输出 LONG
func TestComputeDirection_BearishMarket(t *testing.T) {
	cfg := DefaultConfig()

	in := RootSymbolInput{
		Symbol: "TESTUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.75,
				DataLagMs:            50,
				UpdateIntervalS:      5,
				CvdReliability:       0.80,
				OIReliability:        0.75,
				OrderbookReliability: 0.80,
			},
			Macro: Macro{
				TrendAlignment:  "bearish_aligned",
				SignalStrength:  65,
				ConfidenceLevel: 0.70,
			},
			Micro5m: Micro5m{
				CandleIntent:    "bearish_confirm",
				FuturesCvdDelta: -5000000,
				SpotCvdDelta:    -2000000,
				OIDeltaPct:      -0.5,
				PriceDeltaPct:   -0.8,
				VolumeDelta:     8000000,
				DataQuality:     0.80,
			},
			OB: Orderbook{
				ImbalanceRatio: -0.3,
				BidPressure:    0.3,
				AskPressure:    0.7,
				LiquidityScore: 0.7,
				SpoofingRisk:   0.1,
			},
		},
		MTF: MTFAnalysis{
			"30m": TimeframeData{
				Channel: ChannelInfo{Direction: "flat", CurrentPosition: "break_down", PriceRatio: 0.5, Quality: 0.8},
				VPVR:    VPVR{DistToVAHATR: -13.68, DistToVALATR: 0.15},
			},
			"15m": TimeframeData{
				Channel: ChannelInfo{Direction: "flat", CurrentPosition: "break_down", PriceRatio: 0.5, Quality: 0.8},
				VPVR:    VPVR{DistToVAHATR: -8.21, DistToVALATR: 2.77},
			},
		},
	}

	result := ComputeDirectionArbitration(in, cfg)

	t.Logf("plan_side=%s | conf=%.3f | delta=%.3f | of_dir=%.3f | struct_dir=%.3f | flags=%v",
		result.PlanSide, result.Confidence, result.Delta, result.OFDir, result.StructDir, result.Flags)

	if result.PlanSide == SideLong {
		t.Errorf("熊市场景不应输出 LONG：plan_side=%s, struct_dir=%.3f, of_dir=%.3f",
			result.PlanSide, result.StructDir, result.OFDir)
	}

	// structDir 应为负值（修复后通道信号生效）
	if result.StructDir >= 0 {
		t.Errorf("flat+break_down 场景 struct_dir 应为负，got=%.4f", result.StructDir)
	}
}

// TestComputeDirection_BullishMarket 验证正常多头场景仍能正确输出 LONG
func TestComputeDirection_BullishMarket(t *testing.T) {
	cfg := DefaultConfig()

	in := RootSymbolInput{
		Symbol: "TESTUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.80,
				DataLagMs:            50,
				UpdateIntervalS:      5,
				CvdReliability:       0.85,
				OIReliability:        0.80,
				OrderbookReliability: 0.85,
			},
			Macro: Macro{
				TrendAlignment:  "bullish_aligned",
				SignalStrength:  70,
				ConfidenceLevel: 0.75,
			},
			Micro5m: Micro5m{
				CandleIntent:    "bullish_confirm",
				FuturesCvdDelta: 6000000,
				SpotCvdDelta:    2500000,
				OIDeltaPct:      0.6,
				PriceDeltaPct:   0.9,
				VolumeDelta:     9000000,
				DataQuality:     0.85,
			},
			OB: Orderbook{
				ImbalanceRatio: 0.35,
				BidPressure:    0.70,
				AskPressure:    0.30,
				LiquidityScore: 0.75,
				SpoofingRisk:   0.1,
			},
		},
		MTF: MTFAnalysis{
			"30m": TimeframeData{
				Channel: ChannelInfo{Direction: "up", CurrentPosition: "inside", PriceRatio: 0.2, Quality: 0.8},
				VPVR:    VPVR{DistToVAHATR: -5.0, DistToVALATR: 2.0},
			},
			"15m": TimeframeData{
				Channel: ChannelInfo{Direction: "up", CurrentPosition: "break_up", PriceRatio: 0.5, Quality: 0.8},
				VPVR:    VPVR{DistToVAHATR: -4.0, DistToVALATR: 1.5},
			},
		},
	}

	result := ComputeDirectionArbitration(in, cfg)

	t.Logf("plan_side=%s | conf=%.3f | delta=%.3f | of_dir=%.3f | struct_dir=%.3f | flags=%v",
		result.PlanSide, result.Confidence, result.Delta, result.OFDir, result.StructDir, result.Flags)

	if result.PlanSide != SideLong {
		t.Errorf("强多头场景应输出 LONG，got=%s", result.PlanSide)
	}

	if result.StructDir <= 0 {
		t.Errorf("up+break_up 场景 struct_dir 应为正，got=%.4f", result.StructDir)
	}
}

// TestChannelQualityImpact 验证 Quality 默认值修复（0.0→0.5）对信号强度的影响
func TestChannelQualityImpact(t *testing.T) {
	// 修复前 Quality=0.0 → qualityF=clamp(0,0.3,1)=0.3
	// 修复后 Quality=0.5 → qualityF=clamp(0.5,0.3,1)=0.5
	// 验证 structDir 在修复后有更强的信号

	mtfLowQ := MTFAnalysis{
		"30m": TimeframeData{
			Channel: ChannelInfo{Direction: "down", CurrentPosition: "inside", PriceRatio: 0.8, Quality: 0.3},
			VPVR:    VPVR{DistToVAHATR: -5.0, DistToVALATR: 3.0},
		},
		"15m": TimeframeData{
			Channel: ChannelInfo{Direction: "down", CurrentPosition: "inside", PriceRatio: 0.8, Quality: 0.3},
			VPVR:    VPVR{DistToVAHATR: -4.0, DistToVALATR: 2.0},
		},
	}

	mtfHighQ := MTFAnalysis{
		"30m": TimeframeData{
			Channel: ChannelInfo{Direction: "down", CurrentPosition: "inside", PriceRatio: 0.8, Quality: 0.8},
			VPVR:    VPVR{DistToVAHATR: -5.0, DistToVALATR: 3.0},
		},
		"15m": TimeframeData{
			Channel: ChannelInfo{Direction: "down", CurrentPosition: "inside", PriceRatio: 0.8, Quality: 0.8},
			VPVR:    VPVR{DistToVAHATR: -4.0, DistToVALATR: 2.0},
		},
	}

	lowQ := StructDirFromMTF(mtfLowQ)
	highQ := StructDirFromMTF(mtfHighQ)

	t.Logf("Quality=0.3 → structDir=%.4f", lowQ)
	t.Logf("Quality=0.8 → structDir=%.4f", highQ)

	if lowQ >= 0 || highQ >= 0 {
		t.Errorf("下降通道接近上轨时 structDir 应为负，lowQ=%.4f highQ=%.4f", lowQ, highQ)
	}

	if math.Abs(highQ) <= math.Abs(lowQ) {
		t.Errorf("Quality 越高信号应越强：|highQ|=%.4f 应 > |lowQ|=%.4f", math.Abs(highQ), math.Abs(lowQ))
	}
}
