#!/bin/bash
# 验证常用交易对的TickSize配置

echo "=== 从币安API获取常用交易对TickSize ==="
echo ""

SYMBOLS=("BTCUSDT" "ETHUSDT" "BNBUSDT" "SOLUSDT" "XRPUSDT" "DOGEUSDT" "ADAUSDT" "AVAXUSDT" "DOTUSDT" "MATICUSDT" "LINKUSDT" "UNIUSDT" "ATOMUSDT" "LTCUSDT" "ETCUSDT" "TRXUSDT" "SHIBUSDT" "PEPEUSDT")

for symbol in "${SYMBOLS[@]}"; do
    ticksize=$(curl -s "https://fapi.binance.com/fapi/v1/exchangeInfo" | \
        jq -r ".symbols[] | select(.symbol==\"$symbol\") | .filters[] | select(.filterType==\"PRICE_FILTER\") | .tickSize")

    if [ ! -z "$ticksize" ]; then
        printf "%-15s TickSize: %s\n" "$symbol" "$ticksize"
    fi
done

echo ""
echo "=== 建议的兜底配置 ==="
echo "将以上TickSize添加到 microstructure/exchange_info.go 的 calculateFallbackTickSize 函数中"
