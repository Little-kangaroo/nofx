package triggers

import (
	"math"
	"testing"
)

// ========================================
// 测试工具函数
// ========================================

func TestCalculateATR(t *testing.T) {
	tests := []struct {
		name     string
		klines   []Kline
		period   int
		wantGT   float64 // 期望大于此值
		wantLT   float64 // 期望小于此值
	}{
		{
			name: "正常ATR计算",
			klines: []Kline{
				{High: 100, Low: 95, Close: 98},
				{High: 102, Low: 97, Close: 100},
				{High: 105, Low: 99, Close: 103},
				{High: 104, Low: 100, Close: 102},
				{High: 106, Low: 101, Close: 105},
				{High: 108, Low: 103, Close: 106},
				{High: 107, Low: 104, Close: 105},
				{High: 110, Low: 105, Close: 108},
				{High: 109, Low: 106, Close: 107},
				{High: 112, Low: 107, Close: 110},
				{High: 111, Low: 108, Close: 109},
				{High: 113, Low: 109, Close: 112},
				{High: 115, Low: 110, Close: 113},
				{High: 114, Low: 111, Close: 112},
				{High: 116, Low: 112, Close: 115},
			},
			period: 14,
			wantGT: 3.0,
			wantLT: 6.0,
		},
		{
			name:   "K线数量不足",
			klines: []Kline{{High: 100, Low: 95}},
			period: 14,
			wantGT: -1,
			wantLT: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateATR(tt.klines, tt.period)
			if tt.wantGT >= 0 && (got <= tt.wantGT || got >= tt.wantLT) {
				t.Errorf("CalculateATR() = %v, want between (%v, %v)", got, tt.wantGT, tt.wantLT)
			}
		})
	}
}

func TestCalculateVolumeZScore(t *testing.T) {
	klines := []Kline{
		{Volume: 100}, {Volume: 105}, {Volume: 98}, {Volume: 102},
		{Volume: 101}, {Volume: 99}, {Volume: 103}, {Volume: 100},
		{Volume: 102}, {Volume: 98}, {Volume: 101}, {Volume: 100},
		{Volume: 99}, {Volume: 103}, {Volume: 101}, {Volume: 100},
		{Volume: 102}, {Volume: 98}, {Volume: 101}, {Volume: 100},
	}

	tests := []struct {
		name          string
		currentVolume float64
		expectPos     bool // 期望正Z分数
	}{
		{name: "高量能", currentVolume: 150, expectPos: true},
		{name: "正常量能", currentVolume: 100, expectPos: false},
		{name: "低量能", currentVolume: 50, expectPos: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateVolumeZScore(tt.currentVolume, klines, 20)
			if tt.expectPos && got <= 0 {
				t.Errorf("CalculateVolumeZScore() = %v, expected positive", got)
			}
			if !tt.expectPos && tt.currentVolume > 120 && got <= 0 {
				t.Errorf("CalculateVolumeZScore() = %v, expected positive for high volume", got)
			}
		})
	}
}

// ========================================
// 测试Swing点检测
// ========================================

func TestFindSwingLevels(t *testing.T) {
	// 构造一个明显的Swing High和Swing Low
	klines := []Kline{
		{High: 100, Low: 95},
		{High: 102, Low: 97},
		{High: 105, Low: 100}, // Swing High候选
		{High: 103, Low: 98},
		{High: 101, Low: 96},
		{High: 99, Low: 94},  // Swing Low候选
		{High: 100, Low: 95},
		{High: 102, Low: 97},
		{High: 104, Low: 99},
	}

	cfg := DefaultConfig()
	result := FindSwingLevels(klines, cfg)

	if result.SwingHigh <= 0 {
		t.Errorf("FindSwingLevels() SwingHigh = %v, want > 0", result.SwingHigh)
	}
	if result.SwingLow <= 0 {
		t.Errorf("FindSwingLevels() SwingLow = %v, want > 0", result.SwingLow)
	}
	if result.SwingHigh <= result.SwingLow {
		t.Errorf("FindSwingLevels() SwingHigh(%v) should > SwingLow(%v)", result.SwingHigh, result.SwingLow)
	}

	t.Logf("SwingHigh=%.2f, SwingLow=%.2f", result.SwingHigh, result.SwingLow)
}

// ========================================
// 测试SFP检测
// ========================================

func TestDetectSfpBear(t *testing.T) {
	cfg := DefaultConfig()
	atr := 5.0

	tests := []struct {
		name       string
		k          Kline
		swingHigh  float64
		wantDetect bool
	}{
		{
			name: "典型看空SFP",
			k: Kline{
				High:  105, // 扫过SwingHigh(100)
				Low:   95,
				Open:  98,
				Close: 97, // 收回SwingHigh下方
			},
			swingHigh:  100,
			wantDetect: true,
		},
		{
			name: "未扫高",
			k: Kline{
				High:  99, // 未扫过SwingHigh
				Low:   95,
				Open:  98,
				Close: 97,
			},
			swingHigh:  100,
			wantDetect: false,
		},
		{
			name: "未收回",
			k: Kline{
				High:  105, // 扫过
				Low:   95,
				Open:  98,
				Close: 102, // 但没收回SwingHigh下方
			},
			swingHigh:  100,
			wantDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quality, detected := DetectSfpBear(tt.k, tt.swingHigh, atr, -1, cfg)
			if detected != tt.wantDetect {
				t.Errorf("DetectSfpBear() detected = %v, want %v", detected, tt.wantDetect)
			}
			if detected && (quality < 0 || quality > 1) {
				t.Errorf("DetectSfpBear() quality = %v, want [0, 1]", quality)
			}
			if detected {
				t.Logf("SFP_BEAR detected with quality=%.2f", quality)
			}
		})
	}
}

func TestDetectSfpBull(t *testing.T) {
	cfg := DefaultConfig()
	atr := 5.0

	tests := []struct {
		name       string
		k          Kline
		swingLow   float64
		wantDetect bool
	}{
		{
			name: "典型看多SFP",
			k: Kline{
				High:  110,
				Low:   92, // 扫过SwingLow(95)
				Open:  105,
				Close: 108, // 收回SwingLow上方，且下影线占比足够
			},
			swingLow:   95,
			wantDetect: true,
		},
		{
			name: "未扫低",
			k: Kline{
				High:  105,
				Low:   96, // 未扫过SwingLow
				Open:  97,
				Close: 98,
			},
			swingLow:   95,
			wantDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quality, detected := DetectSfpBull(tt.k, tt.swingLow, atr, -1, cfg)
			if detected != tt.wantDetect {
				t.Errorf("DetectSfpBull() detected = %v, want %v (quality=%.2f)", detected, tt.wantDetect, quality)
			}
			if detected {
				t.Logf("SFP_BULL detected with quality=%.2f", quality)
			}
		})
	}
}

// ========================================
// 测试Engulf检测
// ========================================

func TestDetectEngulfBear(t *testing.T) {
	cfg := DefaultConfig()
	atr := 5.0

	tests := []struct {
		name       string
		k          Kline
		prev       Kline
		wantDetect bool
	}{
		{
			name: "典型看空吞没",
			k: Kline{
				High:  105,
				Low:   95,
				Open:  104, // 阴线
				Close: 96,
			},
			prev: Kline{
				High:  102,
				Low:   98,
				Open:  99, // 小阳线
				Close: 101,
			},
			wantDetect: true,
		},
		{
			name: "非阴线",
			k: Kline{
				High:  105,
				Low:   95,
				Open:  96, // 阳线
				Close: 104,
			},
			prev: Kline{
				High:  102,
				Low:   98,
				Open:  99,
				Close: 101,
			},
			wantDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quality, detected := DetectEngulfBear(tt.k, tt.prev, atr, cfg)
			if detected != tt.wantDetect {
				t.Errorf("DetectEngulfBear() detected = %v, want %v", detected, tt.wantDetect)
			}
			if detected {
				t.Logf("ENGULF_BEAR detected with quality=%.2f", quality)
			}
		})
	}
}

func TestDetectEngulfBull(t *testing.T) {
	cfg := DefaultConfig()
	atr := 5.0

	k := Kline{High: 105, Low: 95, Open: 96, Close: 104} // 阳线吞没
	prev := Kline{High: 102, Low: 98, Open: 101, Close: 99}

	quality, detected := DetectEngulfBull(k, prev, atr, cfg)
	if !detected {
		t.Errorf("DetectEngulfBull() should detect bullish engulfing")
	}
	if detected {
		t.Logf("ENGULF_BULL detected with quality=%.2f", quality)
	}
}

// ========================================
// 测试IBB检测
// ========================================

func TestDetectIbbBull(t *testing.T) {
	cfg := DefaultConfig()
	atr := 5.0

	// 构造Inside Bar突破
	prev2 := Kline{High: 105, Low: 95, Open: 97, Close: 103} // 母K线
	prev := Kline{High: 102, Low: 98, Open: 99, Close: 101}  // Inside Bar
	k := Kline{High: 110, Low: 100, Open: 101, Close: 108}   // 向上突破

	quality, detected := DetectIbbBull(k, prev, prev2, atr, -1, cfg)
	if !detected {
		t.Errorf("DetectIbbBull() should detect inside bar breakout")
	}
	if detected {
		t.Logf("IBB_BULL detected with quality=%.2f", quality)
	}
}

func TestDetectIbbBear(t *testing.T) {
	cfg := DefaultConfig()
	atr := 5.0

	prev2 := Kline{High: 105, Low: 95, Open: 97, Close: 103}
	prev := Kline{High: 102, Low: 98, Open: 99, Close: 101} // Inside Bar
	k := Kline{High: 100, Low: 90, Open: 99, Close: 92}     // 向下突破

	quality, detected := DetectIbbBear(k, prev, prev2, atr, -1, cfg)
	if !detected {
		t.Errorf("DetectIbbBear() should detect inside bar breakout")
	}
	if detected {
		t.Logf("IBB_BEAR detected with quality=%.2f", quality)
	}
}

// ========================================
// 测试Momo检测
// ========================================

func TestDetectMomoBull(t *testing.T) {
	cfg := DefaultConfig()
	atr := 5.0
	volZ := 1.5 // 高量能

	k := Kline{
		High:   115,
		Low:    100, // 波幅15 > 1.2*atr(5) = 6
		Open:   101,
		Close:  113, // 收盘靠近高点
		Volume: 200,
	}

	quality, detected := DetectMomoBull(k, atr, volZ, cfg)
	if !detected {
		t.Errorf("DetectMomoBull() should detect momentum ignition")
	}
	if detected {
		t.Logf("MOMO_BULL detected with quality=%.2f", quality)
	}
}

func TestDetectMomoBear(t *testing.T) {
	cfg := DefaultConfig()
	atr := 5.0
	volZ := 1.5

	k := Kline{
		High:   115,
		Low:    100,
		Open:   113,
		Close:  102, // 收盘靠近低点
		Volume: 200,
	}

	quality, detected := DetectMomoBear(k, atr, volZ, cfg)
	if !detected {
		t.Errorf("DetectMomoBear() should detect momentum ignition")
	}
	if detected {
		t.Logf("MOMO_BEAR detected with quality=%.2f", quality)
	}
}

// ========================================
// 测试主检测流程
// ========================================

func TestDetectTriggers(t *testing.T) {
	cfg := DefaultConfig()

	// 构造一个包含多个触发器的场景
	klines := make([]Kline, 30)
	for i := 0; i < 30; i++ {
		klines[i] = Kline{
			High:   100 + float64(i%5),
			Low:    95 + float64(i%5),
			Open:   97 + float64(i%5),
			Close:  98 + float64(i%5),
			Volume: 100,
		}
	}

	// 最后一根K线设置为看空SFP（扫高失败）
	klines[29] = Kline{
		High:   110, // 扫高
		Low:    95,
		Open:   98,
		Close:  97, // 收回
		Volume: 150,
	}

	atr := 5.0
	volZ := 1.0

	result := DetectTriggers(klines, atr, volZ, cfg)

	// 验证结果结构
	if result.TF != "5m" {
		t.Errorf("DetectTriggers() TF = %v, want 5m", result.TF)
	}

	if len(result.Flags) > cfg.MaxFlagsPerBar {
		t.Errorf("DetectTriggers() flags count = %v, want <= %v", len(result.Flags), cfg.MaxFlagsPerBar)
	}

	if len(result.Flags) > 0 {
		if result.Primary == FlagNone {
			t.Errorf("DetectTriggers() has flags but Primary=NONE")
		}
		// 验证Primary在Flags中
		found := false
		for _, f := range result.Flags {
			if f == result.Primary {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DetectTriggers() Primary=%v not in Flags=%v", result.Primary, result.Flags)
		}
	}

	// 验证质量评分
	for flag, quality := range result.Quality {
		if quality < 0 || quality > 1 {
			t.Errorf("DetectTriggers() quality[%s] = %v, want [0, 1]", flag, quality)
		}
	}

	t.Logf("DetectTriggers() result: TF=%s, Flags=%v, Primary=%s, Quality=%v",
		result.TF, result.Flags, result.Primary, result.Quality)
}

func TestPostProcessFlags(t *testing.T) {
	cfg := DefaultConfig()

	flags := []string{
		FlagSfpBear,
		FlagEngulfBear,
		FlagMomoBear,
		FlagIbbBear,
	}

	quality := map[string]float64{
		FlagSfpBear:    0.75,
		FlagEngulfBear: 0.60,
		FlagMomoBear:   0.45, // 低于QualityMin(0.55)
		FlagIbbBear:    0.65,
	}

	kept, primary := PostProcessFlags(flags, quality, cfg)

	// 应该过滤掉MOMO_BEAR（质量0.45 < 0.55）
	if len(kept) != 3 {
		t.Errorf("PostProcessFlags() kept count = %v, want 3", len(kept))
	}

	// Primary应该是SFP_BEAR（质量最高0.75）
	if primary != FlagSfpBear {
		t.Errorf("PostProcessFlags() primary = %v, want %v", primary, FlagSfpBear)
	}

	// 验证排序（按质量降序）
	for i := 0; i < len(kept)-1; i++ {
		if quality[kept[i]] < quality[kept[i+1]] {
			t.Errorf("PostProcessFlags() not sorted by quality desc")
		}
	}

	t.Logf("PostProcessFlags() kept=%v, primary=%s", kept, primary)
}

// ========================================
// 边界测试
// ========================================

func TestDetectTriggersEmpty(t *testing.T) {
	cfg := DefaultConfig()
	result := DetectTriggers([]Kline{}, 5.0, 1.0, cfg)

	if len(result.Flags) != 0 {
		t.Errorf("DetectTriggers(empty) should have no flags")
	}
	if result.Primary != FlagNone {
		t.Errorf("DetectTriggers(empty) Primary should be NONE")
	}
}

func TestDetectTriggersInvalidATR(t *testing.T) {
	cfg := DefaultConfig()
	klines := []Kline{
		{High: 100, Low: 95},
		{High: 102, Low: 97},
		{High: 105, Low: 100},
	}

	result := DetectTriggers(klines, 0, 1.0, cfg) // ATR=0

	if len(result.Flags) != 0 {
		t.Errorf("DetectTriggers(invalid ATR) should have no flags")
	}
}

// ========================================
// 工具函数测试
// ========================================

func TestClamp(t *testing.T) {
	tests := []struct {
		x, lo, hi, want float64
	}{
		{0.5, 0, 1, 0.5},
		{-0.5, 0, 1, 0},
		{1.5, 0, 1, 1},
		{0.8, 0.2, 0.6, 0.6},
	}

	for _, tt := range tests {
		got := clamp(tt.x, tt.lo, tt.hi)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("clamp(%v, %v, %v) = %v, want %v", tt.x, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestNormalizeScore(t *testing.T) {
	tests := []struct {
		value, min, max, want float64
	}{
		{0.5, 0, 1, 0.5},
		{0, 0, 1, 0},
		{1, 0, 1, 1},
		{0.75, 0.5, 1, 0.5},
		{2, 0, 1, 1}, // Clamp to 1
	}

	for _, tt := range tests {
		got := normalizeScore(tt.value, tt.min, tt.max)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("normalizeScore(%v, %v, %v) = %v, want %v", tt.value, tt.min, tt.max, got, tt.want)
		}
	}
}
