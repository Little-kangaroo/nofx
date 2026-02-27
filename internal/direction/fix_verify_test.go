package direction

import (
	"math"
	"testing"
)

// ========== Bug1/2/3 修复验证：channelSign 和 structDir ==========

// TestChannelSignUnderscoreFormat 验证 channelSign 的各种格式兼容性和修复后的差异化打分
func TestChannelSignUnderscoreFormat(t *testing.T) {
	cases := []struct {
		name      string
		direction string
		position  string
		wantSign  float64
	}{
		// 顺势突破：满分
		{"up+break_up 顺势满分", "up", "break_up", 1.0},
		{"down+break_down 顺势满分", "down", "break_down", -1.0},
		// 横盘突破：60%（修复后，原为 ±1.0）
		{"flat+break_up 横盘 60%", "flat", "break_up", 0.6},
		{"flat+break_down 横盘 -60%", "flat", "break_down", -0.6},
		// 逆势突破：30%（修复后）
		{"down+break_up 逆势 30%", "down", "break_up", 0.3},
		{"up+break_down 逆势 -30%", "up", "break_down", -0.3},
		// 兼容无下划线格式
		{"flat+breakup 横盘 60%", "flat", "breakup", 0.6},
		{"flat+breakdown 横盘 -60%", "flat", "breakdown", -0.6},
		// inside 仍然正常
		{"up+inside 接近下轨 应为 +1.0", "up", "inside", 1.0},
		{"down+inside 接近上轨 应为 -1.0", "down", "inside", -1.0},
		// lower/upper 不变
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

// TestStructDirFromMTF_DownBreakUp 验证逆势 break_up 的折扣处理
// 下降通道 + break_up：应产生弱正信号（0.3），而非原来的满分 +1.0
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
	// 修复后：down+break_up=0.3，flat+break_up=0.6
	// 30m: 0.3×0.7×0.60=0.126，15m: 0.6×0.6×0.40=0.144，sum=0.270
	// 不应再产生 +1.0 的满分多头信号
	if got >= 0.8 {
		t.Errorf("down+break_up 逆势场景不应给出接近满分信号，got=%.4f（期望 < 0.8）", got)
	}
	t.Logf("down+break_up structDir = %.4f（修复后折扣，原为 +1.0）", got)
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

// ========== 亏损记录回归测试（Task #7）==========

// TestLossRecord_BNB_DivergentMixed 复现亏损8.7%的 BNBUSDT 场景
// 修复后，divergent_mixed + bearish_distribution + 空头现货CVD 应触发 MACRO_OPPOSE_BLOCK
func TestLossRecord_BNB_DivergentMixed(t *testing.T) {
	cfg := DefaultConfig()

	in := RootSymbolInput{
		Symbol: "BNBUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         1.0,
				DataLagMs:            5,
				UpdateIntervalS:      300,
				CvdReliability:       1.0,
				OIReliability:        1.0,
				OrderbookReliability: 1.0,
			},
			Macro: Macro{
				TrendAlignment:    "divergent_mixed",
				SignalStrength:    80,
				ConfidenceLevel:   0.8,
				DominantDirection: "bearish_distribution",
				SpotCvd1hUSD:      -1450936.522,
				FuturesCvd1hUSD:   289241,
				CvdDivergence:     false,
			},
			Micro5m: Micro5m{
				CandleIntent:    "mixed_signals",
				FuturesCvdDelta: 225924,
				SpotCvdDelta:    -109838.636,
				OIDeltaPct:      0.03,
				PriceDeltaPct:   0.01,
				VolumeDelta:     335763,
				DataQuality:     1.0,
			},
			OB: Orderbook{
				ImbalanceRatio: 0.6868,
				BidPressure:    194336,
				AskPressure:    107756,
				LiquidityScore: 0.7043,
				SpoofingRisk:   0.531,
				SupportWall: &Wall{
					StrengthUSD:    43240,
					DistancePct:    0,
					IsSolid:        false,
					StabilityScore: 0.4379,
					FlickerCount:   266,
				},
			},
		},
		MTF: MTFAnalysis{
			"30m": TimeframeData{
				Channel:       ChannelInfo{Direction: "flat", CurrentPosition: "break_up", PriceRatio: 1, Quality: 1},
				VPVR:          VPVR{DistToVAHATR: -12.1865, DistToVALATR: 5.8754},
				SupertrendDir: "bullish",
			},
			"15m": TimeframeData{
				Channel:       ChannelInfo{Direction: "flat", CurrentPosition: "break_up", PriceRatio: 1, Quality: 1},
				VPVR:          VPVR{DistToVAHATR: -7.2669, DistToVALATR: 13.2351},
				SupertrendDir: "bullish",
			},
			"4h": TimeframeData{
				Channel:       ChannelInfo{Direction: "down", CurrentPosition: "break_up", PriceRatio: 1, Quality: 1},
				VPVR:          VPVR{DistToVAHATR: -49.2443, DistToVALATR: -4.4772},
				SupertrendDir: "bearish", // 4h 超级趋势为空头！
			},
		},
	}

	result := ComputeDirectionArbitration(in, cfg)

	t.Logf("plan_side=%s | block=%v | block_reason=%s | confidence=%.3f | signal_strength=%.3f | delta=%.3f | struct_dir=%.3f | flags=%v",
		result.PlanSide, result.BlockEntry, result.BlockReason, result.Confidence, result.SignalStrength, result.Delta, result.StructDir, result.Flags)

	// 核心断言：修复后此场景必须被阻断或给出空头/中性方向
	if result.PlanSide == SideLong && !result.BlockEntry {
		t.Errorf("❌ 亏损回归失败：强空头宏观场景（divergent_mixed+bearish_distribution+spot_cvd_1h=-145万）仍输出 LONG 且未阻断\n"+
			"  plan_side=%s, block_entry=%v, flags=%v", result.PlanSide, result.BlockEntry, result.Flags)
	}

	// struct_dir 应因 4h 空头约束被压制到 ≤0.5
	if result.StructDir > 0.5 {
		t.Errorf("❌ 4h 空头约束失效：struct_dir=%.4f 应 ≤ 0.5（4h supertrend=bearish）", result.StructDir)
	}

	// signal_strength 应低于满分（delta 不应过大）
	if result.SignalStrength > 0.5 {
		t.Errorf("❌ 信号强度过高：signal_strength=%.4f（空头宏观环境下多头绝对强度不应超过 0.5）", result.SignalStrength)
	}
}

// TestDivergentBias_StrongSpotDominance 验证 computeDivergentBias 的量比推导
func TestDivergentBias_StrongSpotDominance(t *testing.T) {
	// 现货绝对主导（83%），现货方向空头，dominant=bearish_distribution → 应返回 -1
	m := Macro{
		DominantDirection: "bearish_distribution",
		SpotCvd1hUSD:      -1450936,
		FuturesCvd1hUSD:   289241,
	}
	got := computeDivergentBias(m)
	if got >= 0 {
		t.Errorf("强空头现货主导场景应返回负偏置，got=%.2f", got)
	}
	t.Logf("divergent_mixed 偏置（强空头）= %.2f", got)

	// 现货主导+多头，dominant=bullish_accumulation → 应返回 +1
	m2 := Macro{
		DominantDirection: "bullish_accumulation",
		SpotCvd1hUSD:      1800000,
		FuturesCvd1hUSD:   200000,
	}
	got2 := computeDivergentBias(m2)
	if got2 <= 0 {
		t.Errorf("强多头现货主导场景应返回正偏置，got=%.2f", got2)
	}
	t.Logf("divergent_mixed 偏置（强多头）= %.2f", got2)

	// 现货未占主导（60%），信号存疑，应走 dominant_direction 路径
	m3 := Macro{
		DominantDirection: "bearish_distribution",
		SpotCvd1hUSD:      -600000,
		FuturesCvd1hUSD:   400000,
	}
	got3 := computeDivergentBias(m3)
	if got3 >= 0 {
		t.Errorf("dominant=bearish 场景应返回负偏置，got=%.2f", got3)
	}
	t.Logf("divergent_mixed 偏置（dominant主导）= %.2f", got3)
}

// TestStructDir_4hBearishConstraint 验证 4h 空头背景约束（Task #3）
func TestStructDir_4hBearishConstraint(t *testing.T) {
	// 15m/30m: flat+break_up（修复后各 0.6），4h: supertrend=bearish
	// struct_dir 应被 4h 约束压制到 ≤ 0.5
	mtf := MTFAnalysis{
		"30m": TimeframeData{
			Channel:       ChannelInfo{Direction: "flat", CurrentPosition: "break_up", PriceRatio: 1, Quality: 1},
			VPVR:          VPVR{},
			SupertrendDir: "bullish",
		},
		"15m": TimeframeData{
			Channel:       ChannelInfo{Direction: "flat", CurrentPosition: "break_up", PriceRatio: 1, Quality: 1},
			VPVR:          VPVR{},
			SupertrendDir: "bullish",
		},
		"4h": TimeframeData{
			SupertrendDir: "bearish",
		},
	}

	got := StructDirFromMTF(mtf)

	// flat+break_up 修复后 0.6，sum=0.6，4h bearish → cap at 0.5
	if got > 0.5 {
		t.Errorf("4h 空头约束应将 struct_dir 压制到 ≤0.5，got=%.4f", got)
	}
	t.Logf("4h bearish 约束后 struct_dir=%.4f（期望 ≤0.5）", got)
}

// TestConfidence_SignalStrengthDecoupled 验证 confidence 和 signal_strength 的解耦
// confidence=1.0 但 signal_strength 较低时，应能区分弱信号场景
func TestConfidence_SignalStrengthDecoupled(t *testing.T) {
	cfg := DefaultConfig()

	// 构造一个 ofDir 和 structDir 同向但都很弱的场景
	in := RootSymbolInput{
		Symbol: "TESTUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status: "正常", OverallScore: 0.80, DataLagMs: 10,
				UpdateIntervalS: 5, CvdReliability: 0.8, OIReliability: 0.8, OrderbookReliability: 0.8,
			},
			Macro: Macro{TrendAlignment: "divergent_mixed", SignalStrength: 0, ConfidenceLevel: 0},
			Micro5m: Micro5m{
				CandleIntent: "mixed", FuturesCvdDelta: 50000, SpotCvdDelta: -30000,
				OIDeltaPct: 0.02, PriceDeltaPct: 0.01, VolumeDelta: 100000, DataQuality: 0.8,
			},
			OB: Orderbook{
				ImbalanceRatio: 0.55, BidPressure: 150000, AskPressure: 120000,
				LiquidityScore: 0.65, SpoofingRisk: 0.2,
			},
		},
		MTF: MTFAnalysis{
			"30m": TimeframeData{
				Channel: ChannelInfo{Direction: "flat", CurrentPosition: "break_up", PriceRatio: 1, Quality: 0.5},
				VPVR:    VPVR{DistToVAHATR: -5, DistToVALATR: 3},
			},
			"15m": TimeframeData{
				Channel: ChannelInfo{Direction: "flat", CurrentPosition: "break_up", PriceRatio: 1, Quality: 0.5},
				VPVR:    VPVR{DistToVAHATR: -4, DistToVALATR: 2},
			},
		},
	}

	result := ComputeDirectionArbitration(in, cfg)

	t.Logf("confidence=%.3f | signal_strength=%.3f | delta=%.3f | plan_side=%s",
		result.Confidence, result.SignalStrength, result.Delta, result.PlanSide)

	// signal_strength = |delta|，必须始终等于 |delta|
	if math.Abs(result.SignalStrength-math.Abs(result.Delta)) > 1e-9 {
		t.Errorf("signal_strength(%.4f) 应等于 |delta|(%.4f)", result.SignalStrength, math.Abs(result.Delta))
	}

	// 当 confidence=1.0 时，signal_strength 可能较低，两者提供不同维度信息
	t.Logf("解耦验证：confidence=%.3f（相对方向一致性）vs signal_strength=%.3f（绝对力度）",
		result.Confidence, result.SignalStrength)
}
