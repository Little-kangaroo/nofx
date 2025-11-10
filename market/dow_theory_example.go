package main

import (
	"fmt"
	"time"

	"nofx/market"
)

func main() {
	// 演示道氏理论功能
	fmt.Println("道氏理论分析系统演示")
	fmt.Println("===================")

	// 获取BTCUSDT的市场数据并进行道氏理论分析
	data, err := market.Get("BTCUSDT")
	if err != nil {
		fmt.Printf("获取市场数据失败: %v\n", err)
		return
	}

	// 输出完整的分析结果
	fmt.Println(market.Format(data))

	// 单独展示道氏理论分析结果
	if data.DowTheory != nil {
		fmt.Println("\n=== 详细道氏理论分析 ===")
		
		// 显示配置信息
		config := market.GetDowTheoryConfig()
		fmt.Printf("当前配置:\n")
		fmt.Printf("- 摆动点识别周期: %d\n", config.SwingPointConfig.LookbackPeriod)
		fmt.Printf("- 最小强度阈值: %.2f\n", config.SwingPointConfig.MinStrength)
		fmt.Printf("- 趋势线最少触及次数: %d\n", config.TrendLineConfig.MinTouches)
		fmt.Printf("- 突破阈值: %.1f%%\n", config.SignalConfig.BreakoutStrength*100)
		fmt.Printf("- 最小置信度: %.1f%%\n", config.SignalConfig.MinConfidence)
		fmt.Printf("- 最小风险收益比: %.2f\n", config.SignalConfig.RiskRewardMin)
		fmt.Println()

		// 分析摆动点
		if len(data.DowTheory.SwingPoints) > 0 {
			fmt.Printf("摆动点分析 (共%d个):\n", len(data.DowTheory.SwingPoints))
			for i, point := range data.DowTheory.SwingPoints {
				if i >= 10 { // 只显示前10个
					fmt.Printf("... 还有%d个摆动点\n", len(data.DowTheory.SwingPoints)-10)
					break
				}
				status := "待确认"
				if point.Confirmed {
					status = "已确认"
				}
				fmt.Printf("  %d. %s %.4f (强度%.2f, %s) - %s\n", 
					i+1, point.Type, point.Price, point.Strength, status,
					time.UnixMilli(point.Time).Format("01-02 15:04"))
			}
			fmt.Println()
		}

		// 分析趋势线
		if len(data.DowTheory.TrendLines) > 0 {
			fmt.Printf("趋势线分析 (共%d条):\n", len(data.DowTheory.TrendLines))
			for i, line := range data.DowTheory.TrendLines {
				if i >= 5 { // 只显示前5条
					fmt.Printf("... 还有%d条趋势线\n", len(data.DowTheory.TrendLines)-5)
					break
				}
				fmt.Printf("  %d. %s线: 强度%.2f, 触及%d次, 斜率%.6f\n",
					i+1, line.Type, line.Strength, line.Touches, line.Slope)
				
				if len(line.Points) >= 2 {
					start := line.Points[0]
					end := line.Points[len(line.Points)-1]
					fmt.Printf("     从 %.4f (%s) 到 %.4f (%s)\n",
						start.Price, time.UnixMilli(start.Time).Format("01-02 15:04"),
						end.Price, time.UnixMilli(end.Time).Format("01-02 15:04"))
				}
				
				if line.Broken {
					fmt.Printf("     状态: 已突破 (%s)\n", 
						time.UnixMilli(line.BreakTime).Format("01-02 15:04"))
				} else {
					fmt.Printf("     状态: 有效\n")
				}
			}
			fmt.Println()
		}

		// 分析平行通道
		if data.DowTheory.Channel != nil {
			channel := data.DowTheory.Channel
			fmt.Printf("平行通道分析:\n")
			fmt.Printf("  趋势方向: %s\n", channel.Direction)
			fmt.Printf("  通道质量: %.1f%%\n", channel.Quality*100)
			fmt.Printf("  通道宽度: %.1f%%\n", channel.Width*100)
			fmt.Printf("  当前位置: %s\n", channel.CurrentPos)
			fmt.Printf("  价格比例: %.1f%% (0%%=下轨, 100%%=上轨)\n", channel.PriceRatio*100)
			
			currentTime := time.Now().UnixMilli()
			if channel.UpperLine != nil && channel.LowerLine != nil {
				upperPrice := channel.UpperLine.Slope*float64(currentTime) + channel.UpperLine.Intercept
				lowerPrice := channel.LowerLine.Slope*float64(currentTime) + channel.LowerLine.Intercept
				fmt.Printf("  当前通道边界: 上轨 %.4f, 下轨 %.4f\n", upperPrice, lowerPrice)
			}
			fmt.Println()
		}

		// 分析趋势强度
		if data.DowTheory.TrendStrength != nil {
			strength := data.DowTheory.TrendStrength
			fmt.Printf("趋势强度评估:\n")
			fmt.Printf("  总体强度: %.1f%% (%s)\n", strength.Overall, strength.Quality)
			fmt.Printf("  趋势方向: %s\n", strength.Direction)
			fmt.Printf("  短期强度: %.1f%%, 长期强度: %.1f%%\n", 
				strength.ShortTerm, strength.LongTerm)
			fmt.Printf("  动量强度: %.1f%%\n", strength.Momentum)
			fmt.Printf("  一致性评分: %.1f%%\n", strength.Consistency)
			fmt.Printf("  成交量支撑: %.1f%%\n", strength.VolumeSupport)
			fmt.Println()
		}

		// 分析交易信号
		if data.DowTheory.TradingSignal != nil {
			signal := data.DowTheory.TradingSignal
			fmt.Printf("交易信号:\n")
			fmt.Printf("  建议动作: %s\n", signal.Action)
			fmt.Printf("  信号类型: %s\n", signal.Type)
			fmt.Printf("  置信度: %.1f%%\n", signal.Confidence)
			
			if signal.Entry > 0 {
				fmt.Printf("  入场价: %.4f\n", signal.Entry)
			}
			if signal.StopLoss > 0 {
				fmt.Printf("  止损价: %.4f\n", signal.StopLoss)
			}
			if signal.TakeProfit > 0 {
				fmt.Printf("  止盈价: %.4f\n", signal.TakeProfit)
			}
			if signal.RiskReward > 0 {
				fmt.Printf("  风险收益比: %.2f\n", signal.RiskReward)
			}
			
			fmt.Printf("  信号描述: %s\n", signal.Description)
			
			features := []string{}
			if signal.ChannelBased {
				features = append(features, "基于通道")
			}
			if signal.BreakoutBased {
				features = append(features, "基于突破")
			}
			if len(features) > 0 {
				fmt.Printf("  信号特征: %s\n", fmt.Sprintf("[%s]", 
					fmt.Sprintf("%v", features)))
			}
			
			fmt.Printf("  生成时间: %s\n", 
				time.UnixMilli(signal.Timestamp).Format("2006-01-02 15:04:05"))
			fmt.Println()
		}
	} else {
		fmt.Println("道氏理论分析数据不可用")
	}

	// 演示配置修改
	fmt.Println("=== 配置修改演示 ===")
	newConfig := market.GetDowTheoryConfig()
	newConfig.SwingPointConfig.LookbackPeriod = 7 // 修改摆动点识别周期
	newConfig.SignalConfig.MinConfidence = 70.0   // 提高最小置信度要求
	
	market.UpdateDowTheoryConfig(newConfig)
	fmt.Println("配置已更新:")
	fmt.Printf("- 新的摆动点识别周期: %d\n", newConfig.SwingPointConfig.LookbackPeriod)
	fmt.Printf("- 新的最小置信度: %.1f%%\n", newConfig.SignalConfig.MinConfidence)
}