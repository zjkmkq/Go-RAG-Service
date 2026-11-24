package repository

import (
	"fmt"

	"github.com/google/uuid"
)

// Chunk 对应 chunks 表
type Chunk struct {
	ID         string
	DocumentID string
	Filename   string // 检索时 join documents 带出，用于展示来源
	Content    string
	Embedding  string // 向量字符串，格式如 [0.1,0.2,...,0.768]
	ChunkIndex int
	Distance   float64 // 检索时填充的余弦距离
}

// InsertChunk 插入一条文本块
func InsertChunk(docID, content string, embedding string, chunkIndex int) error {
	id := uuid.New().String()
	_, err := DB.Exec(`
		INSERT INTO chunks (id, document_id, content, embedding, chunk_index)
		VALUES ($1, $2, $3, $4::vector, $5)
	`, id, docID, content, embedding, chunkIndex)
	if err != nil {
		return fmt.Errorf("插入chunk失败: %w", err)
	}
	return nil
}

// GetChunksByDocumentID 查某文档的所有文本块
func GetChunksByDocumentID(docID string) ([]Chunk, error) {
	rows, err := DB.Query(`
		SELECT id, document_id, content, chunk_index
		FROM chunks WHERE document_id = $1
		ORDER BY chunk_index
	`, docID)
	if err != nil {
		return nil, fmt.Errorf("查询chunks失败: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.Content, &c.ChunkIndex); err != nil {
			return nil, fmt.Errorf("扫描chunk失败: %w", err)
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// SearchSimilarChunks 向量相似度检索（Top-K）
func SearchSimilarChunks(embedding string, topK int) ([]Chunk, error) {
	rows, err := DB.Query(`
		SELECT c.id, c.document_id, COALESCE(d.filename, '') AS filename,
		       c.content, c.chunk_index,
		       c.embedding <=> $1::vector AS distance
		FROM chunks c
		LEFT JOIN documents d ON d.id = c.document_id
		ORDER BY c.embedding <=> $1::vector
		LIMIT $2
	`, embedding, topK)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		var distance float64
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.Filename, &c.Content, &c.ChunkIndex, &distance); err != nil {
			return nil, fmt.Errorf("扫描检索结果失败: %w", err)
		}
		c.Distance = distance
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// DeleteChunksByDocumentID 删某文档的所有文本块
func DeleteChunksByDocumentID(docID string) error {
	_, err := DB.Exec(`DELETE FROM chunks WHERE document_id = $1`, docID)
	if err != nil {
		return fmt.Errorf("删除chunks失败: %w", err)
	}
	return nil
}
