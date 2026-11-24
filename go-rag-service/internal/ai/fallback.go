package ai

import (
	"context"
	"fmt"

	"go-rag-service/internal/config"
	"go-rag-service/internal/logger"
)

// FallbackProvider 按 strategy 配置分配 Embed/Generate 到对应 provider
type FallbackProvider struct {
	ollama   *OllamaProvider
	api      *APIProvider
	strategy config.StrategyConfig
}

func NewFallbackProvider(cfg *config.Config) *FallbackProvider {
	return &FallbackProvider{
		ollama:   NewOllamaProvider(cfg.Ollama),
		api:      NewAPIProvider(cfg.API),
		strategy: cfg.Strategy,
	}
}

func (f *FallbackProvider) Name() string {
	return fmt.Sprintf("strategy(embed=%s|gen=%s)", f.strategy.Embed, f.strategy.Generate)
}

func (f *FallbackProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	log := logger.Sugar()

	provider := f.pickEmbed()
	vec, err := provider.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("embed via %s: %w", provider.Name(), err)
	}
	log.Debugw("embed", "via", provider.Name())
	return vec, nil
}

func (f *FallbackProvider) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	log := logger.Sugar()

	provider := f.pickGenerate()
	out, err := provider.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("generate via %s: %w", provider.Name(), err)
	}
	log.Debugw("generate", "via", provider.Name())
	return out, nil
}

func (f *FallbackProvider) pickEmbed() Provider {
	if f.strategy.Embed == "api" {
		return f.api
	}
	return f.ollama
}

func (f *FallbackProvider) pickGenerate() Provider {
	if f.strategy.Generate == "api" {
		return f.api
	}
	return f.ollama
}
