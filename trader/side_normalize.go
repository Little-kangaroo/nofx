package trader

import "strings"

// NormalizeInternalSide 统一系统内部 side = "long"/"short"
// 用于数据库存储、内部逻辑判断等场景
func NormalizeInternalSide(side string) string {
	s := strings.TrimSpace(strings.ToLower(side))
	switch s {
	case "long", "l", "buy":
		return "long"
	case "short", "s", "sell":
		return "short"
	case "long_position", "pos_long", "up":
		return "long"
	case "short_position", "pos_short", "down":
		return "short"
	default:
		// 兼容传入 "LONG"/"SHORT"
		if strings.EqualFold(s, "long") {
			return "long"
		}
		if strings.EqualFold(s, "short") {
			return "short"
		}
		return s
	}
}

// NormalizePositionSide 统一交易所 positionSide = "LONG"/"SHORT"
// 用于 Binance API 调用、权威数据查询等场景
func NormalizePositionSide(side string) string {
	s := strings.TrimSpace(side)
	// 允许传入 long/short/LONG/SHORT
	if strings.EqualFold(s, "long") {
		return "LONG"
	}
	if strings.EqualFold(s, "short") {
		return "SHORT"
	}
	if strings.EqualFold(s, "LONG") {
		return "LONG"
	}
	if strings.EqualFold(s, "SHORT") {
		return "SHORT"
	}

	// 允许 BUY/SELL 语义（当 side 被误传）
	if strings.EqualFold(s, "buy") {
		return "LONG"
	}
	if strings.EqualFold(s, "sell") {
		return "SHORT"
	}
	return strings.ToUpper(s)
}