package repository

import (
	"fmt"
	"time"
)

// DocumentInfo 是列出文档时的视图，带上该文档的片段数
type DocumentInfo struct {
	ID         string    `json:"id"`
	Filename   string    `json:"filename"`
	FileType   string    `json:"file_type"`
	FileSize   int64     `json:"file_size"`
	ChunkCount int       `json:"chunk_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// Update: export InsertChunkFull with explicit ID for service layer
func InsertChunkFull(id, docID, content string, embedding string, chunkIndex int) error {
	_, err := DB.Exec(`
		INSERT INTO chunks (id, document_id, content, embedding, chunk_index)
		VALUES ($1, $2, $3, $4::vector, $5)
	`, id, docID, content, embedding, chunkIndex)
	if err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}
	return nil
}

func InsertDocument(filename, fileType string, fileSize int64) (string, error) {
	var id string
	err := DB.QueryRow(`
		INSERT INTO documents (filename, file_type, file_size)
		VALUES ($1, $2, $3)
		RETURNING id
	`, filename, fileType, fileSize).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert document: %w", err)
	}
	return id, nil
}

// ListDocuments 按上传时间倒序返回所有文档及其片段数
func ListDocuments() ([]DocumentInfo, error) {
	rows, err := DB.Query(`
		SELECT d.id, d.filename, d.file_type, d.file_size,
		       COALESCE(c.cnt, 0) AS chunk_count,
		       d.created_at
		FROM documents d
		LEFT JOIN (
			SELECT document_id, COUNT(*) AS cnt FROM chunks GROUP BY document_id
		) c ON c.document_id = d.id
		ORDER BY d.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	docs := make([]DocumentInfo, 0)
	for rows.Next() {
		var d DocumentInfo
		if err := rows.Scan(&d.ID, &d.Filename, &d.FileType, &d.FileSize, &d.ChunkCount, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// DeleteDocument 删除文档，chunks 由外键 ON DELETE CASCADE 一并清掉
func DeleteDocument(id string) (bool, error) {
	res, err := DB.Exec(`DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete document: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete document rows: %w", err)
	}
	return n > 0, nil
}
