package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go-rag-service/internal/config"
)

type APIProvider struct {
	baseURL    string
	apiKey     string
	llmModel   string
	embedModel string
	client     *http.Client
}

func NewAPIProvider(cfg config.APIConfig) *APIProvider {
	return &APIProvider{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.ResolveAPIKey(),
		llmModel:   cfg.LLMModel,
		embedModel: cfg.EmbedModel,
		client: &http.Client{
			Timeout: cfg.TimeoutDuration(),
		},
	}
}

func (a *APIProvider) Name() string { return "api" }

type openAIEmbedReq struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openAIEmbedResp struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (a *APIProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	return a.embed(ctx, text)
}

func (a *APIProvider) embed(ctx context.Context, text string) ([]float64, error) {
	body := openAIEmbedReq{Model: a.embedModel, Input: text}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	a.setHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api embed HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var r openAIEmbedResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("api embed decode: %w", err)
	}
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("api embed: empty response")
	}
	return r.Data[0].Embedding, nil
}

type openAIChatReq struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (a *APIProvider) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := openAIChatReq{
		Model: a.llmModel,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	a.setHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("api chat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("api chat HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var r openAIChatResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("api chat decode: %w", err)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("api chat: empty response")
	}
	return r.Choices[0].Message.Content, nil
}

func (a *APIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
}
