package main

import (
	"fmt"
	"nofx/market"
)

func main() {
	compressor := market.NewUltimateMatrixCompressor("ETHUSDT")
	fmt.Printf("压缩器创建成功: %t\n", compressor \!= nil)
}