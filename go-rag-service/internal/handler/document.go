package handler

import (
	"encoding/json"
	"net/http"

	"go-rag-service/internal/logger"
	"go-rag-service/internal/service"
)

type DocumentHandler struct {
	svc *service.DocumentService
}

func NewDocumentHandler(svc *service.DocumentService) *DocumentHandler {
	return &DocumentHandler{svc: svc}
}

func (h *DocumentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 32 MB)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	log := logger.Sugar()
	log.Infow("upload received", "filename", header.Filename, "size", header.Size)

	docID, err := h.svc.Ingest(r.Context(), header.Filename, file)
	if err != nil {
		log.Errorw("ingest failed", "error", err)
		http.Error(w, "ingest failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]string{"document_id": docID, "filename": header.Filename}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// List 返回知识库中已入库的文档列表：GET /api/v1/documents
func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	docs, err := h.svc.List()
	if err != nil {
		logger.Sugar().Errorw("list documents failed", "error", err)
		http.Error(w, "list failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"documents": docs,
		"total":     len(docs),
	})
}

// Delete 删除指定文档及其片段：DELETE /api/v1/documents?id=<uuid>
func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	ok, err := h.svc.Delete(id)
	if err != nil {
		logger.Sugar().Errorw("delete document failed", "error", err, "id", id)
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}

	logger.Sugar().Infow("document deleted", "id", id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"deleted": id})
}
