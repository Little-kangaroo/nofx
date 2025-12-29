package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// 配置
	dbPath := "/Users/guiling/IdeaProjects/nofx/config.db"
	outputDir := "/Users/guiling/Downloads/nofx"
	targetDate := "2025-12-29" // 只导出这一天的数据

	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	// 打开数据库
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()

	// 查询2025-12-24这天的记录
	// 使用 LIKE 匹配，因为 timestamp 格式为 "2025-12-24 HH:MM:SS.mmmmmmmmm+08:00"
	query := `
		SELECT id, input_prompt, cot_trace, timestamp
		FROM decision_records
		WHERE timestamp LIKE ?
		ORDER BY timestamp ASC
	`

	rows, err := db.Query(query, targetDate+"%")
	if err != nil {
		log.Fatalf("查询数据失败: %v", err)
	}
	defer rows.Close()

	// 遍历结果并写入文件
	fileIndex := 1
	recordCount := 0

	for rows.Next() {
		var id, inputPrompt, cotTrace, timestamp string

		err := rows.Scan(&id, &inputPrompt, &cotTrace, &timestamp)
		if err != nil {
			log.Printf("读取记录失败: %v", err)
			continue
		}

		// 构造文件名
		fileName := fmt.Sprintf("data%d.txt", fileIndex)
		filePath := filepath.Join(outputDir, fileName)

		// 写入文件
		content := fmt.Sprintf("输入数据：\n%s\n\nAI决策:\n%s\n", inputPrompt, cotTrace)
		err = os.WriteFile(filePath, []byte(content), 0644)
		if err != nil {
			log.Printf("写入文件 %s 失败: %v", fileName, err)
			continue
		}

		log.Printf("✅ 已导出记录 %d: %s (ID: %s, 时间: %s)", fileIndex, fileName, id, timestamp)
		fileIndex++
		recordCount++
	}

	if err = rows.Err(); err != nil {
		log.Fatalf("遍历结果集失败: %v", err)
	}

	log.Printf("\n🎉 导出完成！共导出 %d 条记录到 %s", recordCount, outputDir)
}
