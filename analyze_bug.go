package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("=== 测试修复后的cleanupExpiredData逻辑 ===")
	
	// 模拟changes数组
	type OIChange struct {
		Timestamp time.Time
		OpenInterest float64
	}
	
	now := time.Now()
	windowDuration := 24 * time.Hour
	cutoffTime := now.Add(-windowDuration)
	
	fmt.Printf("当前时间: %v\n", now.Format("15:04:05"))
	fmt.Printf("截止时间(24小时前): %v\n", cutoffTime.Format("15:04:05"))
	
	// 测试情况1：所有记录都过期
	changes := []OIChange{
		{Timestamp: now.Add(-25 * time.Hour), OpenInterest: 1000}, // 25小时前 - 过期
		{Timestamp: now.Add(-26 * time.Hour), OpenInterest: 1100}, // 26小时前 - 过期
	}
	
	fmt.Printf("\n情况1：所有记录都过期\n")
	for i, change := range changes {
		fmt.Printf("记录%d时间: %v (过期: %v)\n", i, 
			change.Timestamp.Format("15:04:05"), 
			!change.Timestamp.After(cutoffTime))
	}
	
	// 执行修复后的清理逻辑
	validStart := -1 // 初始化为-1
	for i, change := range changes {
		if change.Timestamp.After(cutoffTime) {
			validStart = i
			break
		}
	}
	
	fmt.Printf("validStart = %d\n", validStart)
	
	if validStart > 0 {
		changes = changes[validStart:]
		fmt.Printf("部分清理：保留从索引%d开始的记录\n", validStart)
	} else if validStart == -1 {
		changes = changes[:0]
		fmt.Printf("完全清理：所有记录都过期，清空数组\n")
	}
	
	fmt.Printf("清理后数组长度: %d\n", len(changes))
	
	// 测试情况2：部分记录过期
	changes2 := []OIChange{
		{Timestamp: now.Add(-25 * time.Hour), OpenInterest: 1000}, // 25小时前 - 过期
		{Timestamp: now.Add(-23 * time.Hour), OpenInterest: 1100}, // 23小时前 - 有效
		{Timestamp: now.Add(-22 * time.Hour), OpenInterest: 1200}, // 22小时前 - 有效
	}
	
	fmt.Printf("\n情况2：部分记录过期\n")
	for i, change := range changes2 {
		fmt.Printf("记录%d时间: %v (过期: %v)\n", i, 
			change.Timestamp.Format("15:04:05"), 
			!change.Timestamp.After(cutoffTime))
	}
	
	validStart2 := -1
	for i, change := range changes2 {
		if change.Timestamp.After(cutoffTime) {
			validStart2 = i
			break
		}
	}
	
	fmt.Printf("validStart = %d\n", validStart2)
	
	if validStart2 > 0 {
		changes2 = changes2[validStart2:]
		fmt.Printf("部分清理：保留从索引%d开始的记录\n", validStart2)
	} else if validStart2 == -1 {
		changes2 = changes2[:0]
		fmt.Printf("完全清理：所有记录都过期，清空数组\n")
	}
	
	fmt.Printf("清理后数组长度: %d\n", len(changes2))
}