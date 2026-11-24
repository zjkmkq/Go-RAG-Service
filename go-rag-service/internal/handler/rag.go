package handler

import (
	"encoding/json"
	"net/http"

	"go-rag-service/internal/logger"
	"go-rag-service/internal/service"
)

type RAGHandler struct {
	svc *service.RAGService
}

func NewRAGHandler(svc *service.RAGService) *RAGHandler {
	return &RAGHandler{svc: svc}
}

type queryReq struct {
	Question string `json:"question"`
	TopK     int    `json:"top_k"`
}

type queryResp struct {
	Answer  string             `json:"answer"`
	Sources []service.RAGSource `json:"sources"`
}

func (h *RAGHandler) Query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req queryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Question == "" {
		http.Error(w, "question is required", http.StatusBadRequest)
		return
	}

	if req.TopK <= 0 {
		req.TopK = 5
	}

	log := logger.Sugar()
	log.Infow("rag query", "question", req.Question, "topK", req.TopK)

	result, err := h.svc.Query(r.Context(), req.Question, req.TopK)
	if err != nil {
		log.Errorw("rag query failed", "error", err)
		http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := queryResp{
		Answer:  result.Answer,
		Sources: result.Sources,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorw("encode response", "error", err)
	}
}
