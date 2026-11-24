package service

import (
	"context"
	"fmt"
	"strings"

	"go-rag-service/internal/ai"
	"go-rag-service/internal/logger"
	"go-rag-service/internal/repository"
)

type RAGService struct {
	provider ai.Provider
}

func NewRAGService(p ai.Provider) *RAGService {
	return &RAGService{provider: p}
}

type RAGResult struct {
	Answer  string
	Sources []RAGSource
}

type RAGSource struct {
	DocumentID string  `json:"document_id"`
	Filename   string  `json:"filename"`
	Content    string  `json:"content"`
	Distance   float64 `json:"distance"`
}

func (s *RAGService) Query(ctx context.Context, question string, topK int) (*RAGResult, error) {
	log := logger.Sugar()

	// 1. Embed the question
	vec, err := s.provider.Embed(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// 2. Search similar chunks
	embeddingStr := vectorToString(vec)
	chunks, err := repository.SearchSimilarChunks(embeddingStr, topK)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}

	if len(chunks) == 0 {
		return &RAGResult{
			Answer: "No relevant content found in the knowledge base.",
		}, nil
	}

	log.Debugw("retrieved chunks", "count", len(chunks))

	// 3. Build prompt with context
	var contextBuilder strings.Builder
	for i, c := range chunks {
		contextBuilder.WriteString(fmt.Sprintf("[%d] %s\n", i+1, c.Content))
	}

	systemPrompt := "You are a helpful assistant. Answer the user's question based only on the provided context. " +
		"If the context doesn't contain the answer, say so honestly. Keep answers concise."

	userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", contextBuilder.String(), question)

	// 4. Generate answer
	answer, err := s.provider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	// 5. Collect sources
	sources := make([]RAGSource, 0, len(chunks))
	for _, c := range chunks {
		display := c.Content
		// 按字符（rune）截断，避免切坏中文的多字节序列
		if r := []rune(display); len(r) > 500 {
			display = string(r[:500]) + "..."
		}
		sources = append(sources, RAGSource{
			DocumentID: c.DocumentID,
			Filename:   c.Filename,
			Content:    display,
			Distance:   c.Distance,
		})
	}

	return &RAGResult{Answer: answer, Sources: sources}, nil
}
