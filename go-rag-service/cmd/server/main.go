package main

import (
	"fmt"
	"net/http"
	"os"

	"go-rag-service/internal/ai"
	"go-rag-service/internal/config"
	"go-rag-service/internal/handler"
	"go-rag-service/internal/logger"
	"go-rag-service/internal/repository"
	"go-rag-service/internal/service"
)

func main() {
	// 1. Load config
	cfg := config.Load()

	// 2. Init logger
	logger.Init()
	defer logger.Sync()
	sugar := logger.Sugar()

	// 3. Init database
	db, err := repository.InitDB(cfg)
	if err != nil {
		sugar.Errorf("PostgreSQL not reachable at %s:%d: %v", cfg.Database.Host, cfg.Database.Port, err)
		sugar.Infof("Hint: start it with 'docker compose up -d' (docker-compose.yml in project root)")
		logger.Sync()
		os.Exit(1)
	}
	defer repository.CloseDB()

	if err := repository.RunMigrations(db); err != nil {
		sugar.Fatalf("migration failed: %v", err)
	}
	sugar.Info("database ready")

	// 4. Init AI provider (local-first with API fallback)
	provider := ai.NewFallbackProvider(cfg)
	sugar.Infof("AI provider: %s", provider.Name())

	// 5. Init services
	docSvc := service.NewDocumentService(provider)
	ragSvc := service.NewRAGService(provider)

	// 6. Init handlers
	docHandler := handler.NewDocumentHandler(docSvc)
	ragHandler := handler.NewRAGHandler(ragSvc)

	// 7. Routes
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	http.HandleFunc("/api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			docHandler.List(w, r)
		case http.MethodPost:
			docHandler.Upload(w, r)
		case http.MethodDelete:
			docHandler.Delete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	http.HandleFunc("/api/v1/rag", ragHandler.Query)

	// 8. Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	sugar.Infof("server starting on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		sugar.Fatalf("server failed: %v", err)
	}
}
