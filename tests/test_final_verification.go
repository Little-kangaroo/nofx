package test

import (
	"fmt"
	"nofx/market"
)

func TestMatrixCompressor() {
	compressor := market.NewUltimateMatrixCompressor("ETHUSDT")
	fmt.Printf("压缩器创建成功: %t\n", compressor != nil)
}