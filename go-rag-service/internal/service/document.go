package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"go-rag-service/internal/ai"
	"go-rag-service/internal/logger"
	"go-rag-service/internal/repository"

	"github.com/google/uuid"
)

type DocumentService struct {
	provider ai.Provider
}

func NewDocumentService(p ai.Provider) *DocumentService {
	return &DocumentService{provider: p}
}

func (s *DocumentService) Ingest(ctx context.Context, filename string, reader io.Reader) (string, error) {
	log := logger.Sugar()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	content := string(raw)

	// Determine file type from extension
	fileType := detectFileType(filename)

	// Insert document record
	docID, err := repository.InsertDocument(filename, fileType, int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("create document: %w", err)
	}

	// Chunk the text
	chunks := chunkText(content, 500)
	log.Infow("document chunked", "docID", docID, "chunks", len(chunks))

	// Embed + store each chunk
	for i, chunk := range chunks {
		vec, err := s.provider.Embed(ctx, chunk)
		if err != nil {
			return "", fmt.Errorf("embed chunk %d: %w", i, err)
		}

		embeddingStr := vectorToString(vec)

		chunkID := uuid.New().String()
		if err := repository.InsertChunkFull(chunkID, docID, chunk, embeddingStr, i); err != nil {
			return "", fmt.Errorf("store chunk %d: %w", i, err)
		}
	}

	log.Infow("document ingested", "docID", docID, "chunks", len(chunks))
	return docID, nil
}

// List 返回知识库中所有文档及其片段数
func (s *DocumentService) List() ([]repository.DocumentInfo, error) {
	return repository.ListDocuments()
}

// Delete 删除指定文档及其全部片段
func (s *DocumentService) Delete(id string) (bool, error) {
	return repository.DeleteDocument(id)
}

// chunkText 切分文本。
// Markdown（含 # 标题）：按标题切分，标题与其下属正文/表格保持在同一块，单块超长再硬切。
// 纯文本：按空行分段，累积到 maxChars 上限即切。
func chunkText(text string, maxChars int) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}

	lines := strings.Split(text, "\n")

	// 标记真正的标题行：代码围栏（``` / ~~~）内部的 # 是注释，不是标题
	headingLevel := make([]int, len(lines))
	levelCount := map[int]int{}
	hasHeading := false
	inFence := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence {
			if lv := markdownHeadingLevel(l); lv > 0 {
				headingLevel[i] = lv
				levelCount[lv]++
				hasHeading = true
			}
		}
	}

	// 切分粒度：取「出现至少两次」的最浅标题层级。
	// 例如文档有多个 ## 和若干个 ###，就按 ## 切，### 归入父块，
	// 避免「### 2. 上传文档」这种子节被切成孤立小块。
	splitLevel := 7
	for lv := 1; lv <= 6; lv++ {
		if levelCount[lv] >= 2 {
			splitLevel = lv
			break
		}
	}
	if splitLevel == 7 {
		for lv := 1; lv <= 6; lv++ {
			if levelCount[lv] > 0 {
				splitLevel = lv
				break
			}
		}
	}

	var chunks []string
	var cur []string
	curLen := 0

	flush := func() {
		if len(cur) == 0 {
			return
		}
		if s := strings.TrimSpace(strings.Join(cur, "\n")); s != "" {
			chunks = append(chunks, s)
		}
		cur = nil
		curLen = 0
	}

	for i, raw := range lines {
		line := strings.TrimRight(raw, " \t")

		// 单行超过上限（如压缩过的长段落）：直接硬切成独立块
		for utf8.RuneCountInString(line) > maxChars {
			flush()
			r := []rune(line)
			chunks = append(chunks, strings.TrimSpace(string(r[:maxChars])))
			line = string(r[maxChars:])
		}

		// 遇到「达到切分层级」的标题：另起一块；更深的子标题留在父块内
		if hasHeading && headingLevel[i] > 0 && headingLevel[i] <= splitLevel {
			flush()
			cur = append(cur, line)
			curLen = utf8.RuneCountInString(line) + 1
			continue
		}

		// 纯文本模式下空行才分段；Markdown 模式下空行只是排版，不切
		if !hasHeading && strings.TrimSpace(line) == "" {
			flush()
			continue
		}

		lineLen := utf8.RuneCountInString(line) + 1
		if curLen > 0 && curLen+lineLen > maxChars {
			flush()
		}

		cur = append(cur, line)
		curLen += lineLen
	}
	flush()

	return mergeHeadingOnly(chunks)
}

// mergeHeadingOnly 把「只有标题、没有正文」的块并入下一块，
// 避免 ## 父标题（后面紧跟 ### 子标题）单独成块变成零信息碎片。
func mergeHeadingOnly(chunks []string) []string {
	if len(chunks) == 0 {
		return chunks
	}

	out := make([]string, 0, len(chunks))
	var pending []string

	for _, c := range chunks {
		if hasBody(c) {
			if len(pending) > 0 {
				c = strings.Join(pending, "\n") + "\n" + c
				pending = nil
			}
			out = append(out, c)
			continue
		}
		pending = append(pending, c)
	}

	if len(pending) > 0 {
		if len(out) > 0 {
			out[len(out)-1] += "\n" + strings.Join(pending, "\n")
		} else {
			out = append(out, strings.Join(pending, "\n"))
		}
	}

	return out
}

// hasBody 判断块内除标题外是否还有实际内容（代码围栏内的内容算正文）
func hasBody(chunk string) bool {
	inFence := false
	for _, l := range strings.Split(chunk, "\n") {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !isMarkdownHeading(l) {
			return true
		}
	}
	return false
}

// markdownHeadingLevel 返回 ATX 标题层级（1~6），非标题返回 0
func markdownHeadingLevel(line string) int {
	t := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(t, "#") {
		return 0
	}
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return 0
	}
	if n == len(t) || t[n] == ' ' || t[n] == '\t' {
		return n
	}
	return 0
}

// isMarkdownHeading 判断是否为 ATX 标题行（# ~ ######）
func isMarkdownHeading(line string) bool {
	return markdownHeadingLevel(line) > 0
}


func detectFileType(filename string) string {
	if idx := strings.LastIndex(filename, "."); idx != -1 {
		return strings.ToLower(filename[idx+1:])
	}
	return "txt"
}

func vectorToString(vec []float64) string {
	s := "["
	for i, v := range vec {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%f", v)
	}
	s += "]"
	return s
}
