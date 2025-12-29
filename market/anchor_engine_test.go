package market

import (
	"math"
	"testing"
)

// TestEffectiveDistance 测试effectiveDistance函数的各种边界情况
func TestEffectiveDistance(t *testing.T) {
	tests := []struct {
		name     string
		price    float64
		bandLo   float64
		bandHi   float64
		expected float64
		desc     string
	}{
		{
			name:     "Point_Structure_Above",
			price:    100.0,
			bandLo:   95.0,
			bandHi:   95.0, // 点位结构
			expected: 5.0,
			desc:     "点位结构，价格在上方",
		},
		{
			name:     "Point_Structure_Below",
			price:    90.0,
			bandLo:   95.0,
			bandHi:   95.0, // 点位结构
			expected: 5.0,
			desc:     "点位结构，价格在下方",
		},
		{
			name:     "Point_Structure_Exact",
			price:    95.0,
			bandLo:   95.0,
			bandHi:   95.0,
			expected: 0.0,
			desc:     "点位结构，价格恰好在点位",
		},
		{
			name:     "Zone_Price_Inside",
			price:    97.0,
			bandLo:   95.0,
			bandHi:   100.0,
			expected: 0.0,
			desc:     "Zone结构，价格在Zone内部",
		},
		{
			name:     "Zone_Price_Below",
			price:    90.0,
			bandLo:   95.0,
			bandHi:   100.0,
			expected: 5.0, // 到BandLo的距离
			desc:     "Zone结构，价格在Zone下方",
		},
		{
			name:     "Zone_Price_Above",
			price:    105.0,
			bandLo:   95.0,
			bandHi:   100.0,
			expected: 5.0, // 到BandHi的距离
			desc:     "Zone结构，价格在Zone上方",
		},
		{
			name:     "Zone_Price_On_Lower_Bound",
			price:    95.0,
			bandLo:   95.0,
			bandHi:   100.0,
			expected: 0.0,
			desc:     "Zone结构，价格恰好在下沿",
		},
		{
			name:     "Zone_Price_On_Upper_Bound",
			price:    100.0,
			bandLo:   95.0,
			bandHi:   100.0,
			expected: 0.0,
			desc:     "Zone结构，价格恰好在上沿",
		},
		{
			name:     "Zone_Inverted_Bands",
			price:    105.0,
			bandLo:   100.0, // 故意颠倒
			bandHi:   95.0,
			expected: 5.0, // 应该自动交换后计算
			desc:     "Zone结构，BandLo>BandHi（数据错误，自动修正）",
		},
		{
			name:     "Real_Demand_Zone_ETH",
			price:    2932.74,
			bandLo:   2911.06,
			bandHi:   2926.99,
			expected: 5.75, // price在上方，到BandHi距离
			desc:     "真实案例：ETHUSDT需求区",
		},
		{
			name:     "Real_Supply_Zone_ETH",
			price:    2932.74,
			bandLo:   2956.33,
			bandHi:   2975.34,
			expected: 23.59, // price在下方，到BandLo距离
			desc:     "真实案例：ETHUSDT供给区",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := AnchorCandidate{
				BandLo: tt.bandLo,
				BandHi: tt.bandHi,
				Level:  (tt.bandLo + tt.bandHi) / 2, // Level不应被使用
			}

			result := effectiveDistance(tt.price, candidate)

			// 使用浮点容忍度比较
			if math.Abs(result-tt.expected) > 1e-6 {
				t.Errorf("%s 失败:\n  描述: %s\n  价格: %.2f, BandLo: %.2f, BandHi: %.2f\n  期望: %.2f\n  实际: %.2f",
					tt.name, tt.desc, tt.price, tt.bandLo, tt.bandHi, tt.expected, result)
			} else {
				t.Logf("✅ %s 通过: %.2f (描述: %s)", tt.name, result, tt.desc)
			}
		})
	}
}

// TestEffectiveDistance_vs_LevelDistance 对比测试：有效距离 vs 传统Level距离
func TestEffectiveDistance_vs_LevelDistance(t *testing.T) {
	testCases := []struct {
		name           string
		price          float64
		bandLo         float64
		bandHi         float64
		expectImproved bool // 期望有效距离更准确（通常更小）
		desc           string
	}{
		{
			name:           "Price_Near_Zone_Edge",
			price:          2932.0,
			bandLo:         2911.0,
			bandHi:         2927.0,
			expectImproved: true, // 价格靠近上沿，有效距离=5，Level距离=13
			desc:           "价格接近Zone上沿，有效距离应更小",
		},
		{
			name:           "Price_Far_From_Zone",
			price:          2900.0,
			bandLo:         2950.0,
			bandHi:         2975.0,
			expectImproved: false, // 价格远离Zone，两种方法差异小
			desc:           "价格远离Zone，两种方法应接近",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := AnchorCandidate{
				BandLo: tc.bandLo,
				BandHi: tc.bandHi,
				Level:  (tc.bandLo + tc.bandHi) / 2,
			}

			effectiveDist := effectiveDistance(tc.price, candidate)
			levelDist := math.Abs(tc.price - candidate.Level)

			t.Logf("价格: %.2f, Zone: [%.2f - %.2f], Level: %.2f",
				tc.price, tc.bandLo, tc.bandHi, candidate.Level)
			t.Logf("  有效距离: %.2f", effectiveDist)
			t.Logf("  Level距离: %.2f", levelDist)

			if tc.expectImproved {
				if effectiveDist >= levelDist {
					t.Errorf("期望有效距离更小，但实际: 有效距离=%.2f >= Level距离=%.2f",
						effectiveDist, levelDist)
				} else {
					t.Logf("✅ 有效距离改进：%.2f < %.2f (改善 %.1f%%)",
						effectiveDist, levelDist, (1-effectiveDist/levelDist)*100)
				}
			}
		})
	}
}
