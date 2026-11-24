package repository

import (
	"database/sql"
	"fmt"
)

// embeddingDim 是 chunks.embedding 的向量维度，需与嵌入模型输出一致。
// qwen3-embed:0.6b / bge-large-zh-v1.5 都是 1024 维。
const embeddingDim = 1024

func RunMigrations(db *sql.DB) error {
	// 1. Enable pgvector
	if _, err := db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return err
	}

	// 2. Documents table
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS documents (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			filename VARCHAR(255) NOT NULL,
			file_type VARCHAR(50) NOT NULL,
			file_size BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`); err != nil {
		return err
	}

	// 3. Chunks table（仅在维度不匹配时重建，不再每次启动都清库）
	return ensureChunksTable(db)
}

// ensureChunksTable 保证 chunks 表存在且向量维度正确。
//
// 历史行为是每次启动都 DROP TABLE chunks CASCADE，等于服务一重启知识库就清空，
// 测试时必须把文档全部重新上传。这里改为：维度一致就保留数据，
// 只有换嵌入模型导致维度变化时才重建（此时旧向量本来也无法使用）。
func ensureChunksTable(db *sql.DB) error {
	var typ string
	err := db.QueryRow(`
		SELECT format_type(atttypid, atttypmod)
		FROM pg_attribute
		WHERE attrelid = to_regclass('chunks')
		  AND attname = 'embedding'
		  AND NOT attisdropped
	`).Scan(&typ)

	switch {
	case err == sql.ErrNoRows:
		// 表不存在，走创建分支
	case err != nil:
		return fmt.Errorf("inspect chunks table: %w", err)
	default:
		if typ == fmt.Sprintf("vector(%d)", embeddingDim) {
			return nil // 维度和现有表结构一致，保留已有数据
		}
		// 维度变了，旧向量无法使用，重建
		if _, err := db.Exec(`DROP TABLE IF EXISTS chunks CASCADE`); err != nil {
			return fmt.Errorf("drop chunks for dim change: %w", err)
		}
	}

	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS chunks (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			content TEXT NOT NULL,
			embedding vector(%d),
			chunk_index INTEGER NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`, embeddingDim)); err != nil {
		return fmt.Errorf("create chunks table: %w", err)
	}
	return nil
}
