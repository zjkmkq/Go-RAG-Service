package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go-rag-service/internal/config"
)

type OllamaProvider struct {
	host       string
	llmModel   string
	embedModel string
	client     *http.Client
}

func NewOllamaProvider(cfg config.OllamaConfig) *OllamaProvider {
	return &OllamaProvider{
		host:       cfg.Host,
		llmModel:   cfg.LLMModel,
		embedModel: cfg.EmbedModel,
		client: &http.Client{
			Timeout: cfg.TimeoutDuration(),
		},
	}
}

func (o *OllamaProvider) Name() string { return "ollama" }

type ollamaEmbedReq struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResp struct {
	Embedding []float64 `json:"embedding"`
}

func (o *OllamaProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	body := ollamaEmbedReq{Model: o.embedModel, Prompt: text}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.host+"/api/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var r ollamaEmbedResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	return r.Embedding, nil
}

type ollamaChatReq struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResp struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func (o *OllamaProvider) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := ollamaChatReq{
		Model:  o.llmModel,
		Stream: false,
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.host+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama chat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama chat HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var r ollamaChatResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("ollama chat decode: %w", err)
	}
	return r.Message.Content, nil
}

func init() { _ = time.Now }
