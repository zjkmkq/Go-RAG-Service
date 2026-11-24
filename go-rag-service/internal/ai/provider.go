package ai

import "context"

// Provider 统一抽象：embedding + chat 生成
type Provider interface {
	Name() string
	Embed(ctx context.Context, text string) ([]float64, error)
	Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

type Generator interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// chunkResult 是 RAG 检索中间结果
type ChunkResult struct {
	Content  string
	Distance float64
}
