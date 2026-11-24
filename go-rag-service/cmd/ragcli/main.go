package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------- 后端响应结构 ----------

type docInfo struct {
	ID         string `json:"id"`
	Filename   string `json:"filename"`
	FileType   string `json:"file_type"`
	FileSize   int64  `json:"file_size"`
	ChunkCount int    `json:"chunk_count"`
}

type listResp struct {
	Documents []docInfo `json:"documents"`
	Total     int       `json:"total"`
}

type source struct {
	DocumentID string  `json:"document_id"`
	Filename   string  `json:"filename"`
	Content    string  `json:"content"`
	Distance   float64 `json:"distance"`
}

type askResp struct {
	Answer  string   `json:"answer"`
	Sources []source `json:"sources"`
}

// ---------- 运行状态 ----------

var (
	serverURL string
	topK      = 3
	docsCache []docInfo
	client    = &http.Client{Timeout: 10 * time.Minute}
)

func main() {
	flag.StringVar(&serverURL, "server", "http://localhost:8080", "服务地址")
	flag.IntVar(&topK, "topk", 3, "每次提问召回的片段数")
	flag.Parse()
	serverURL = strings.TrimRight(serverURL, "/")

	initConsoleUTF8()

	if err := checkHealth(); err != nil {
		fmt.Printf("连不上服务：%s\n\n", err)
		fmt.Println("先在项目根目录启动服务：")
		fmt.Println("    cd go-rag-service")
		fmt.Println("    go run ./cmd/server")
		fmt.Println()
		fmt.Println("Windows 下也可双击 go-rag-service/启动服务.bat，")
		fmt.Println("它会依次拉起数据库容器、Ollama 和服务后再进入问答。")
		os.Exit(1)
	}

	printBanner()
	refreshDocs()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Print("\n> ")
		if !sc.Scan() {
			fmt.Println("\n再见。")
			return
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		switch strings.ToLower(line) {
		case "exit", "quit", "q", "退出":
			fmt.Println("再见。")
			return
		case "help", "h", "?", "帮助":
			printHelp()
			continue
		case "docs", "ls", "文件":
			refreshDocs()
			printDocs()
			continue
		}

		if cmd, arg, ok := splitCmd(line); ok {
			switch cmd {
			case "rm", "del", "删除":
				deleteDoc(arg)
				continue
			case "topk":
				setTopK(arg)
				continue
			}
		}

		// 是文件路径就上传，否则当作问题
		if p, ok := existingFile(line); ok {
			upload(p)
			continue
		}
		ask(line)
	}
}

// splitCmd 解析 "rm 2" / "topk 5" 这类带参命令
func splitCmd(line string) (string, string, bool) {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 0 {
		return "", "", false
	}
	cmd := strings.ToLower(strings.TrimSpace(parts[0]))
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}
	return cmd, arg, true
}

// existingFile 判断输入是否为磁盘上存在的文件。
// Windows 终端拖放文件会得到被引号包裹的路径，这里一并去掉引号。
func existingFile(line string) (string, bool) {
	p := strings.Trim(strings.TrimSpace(line), `"'`)
	p = strings.TrimRight(p, "\r")
	if p == "" {
		return "", false
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return "", false
	}
	return p, true
}

// ---------- HTTP ----------

func checkHealth() error {
	resp, err := client.Get(serverURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health 返回 %d", resp.StatusCode)
	}
	return nil
}

func refreshDocs() {
	resp, err := client.Get(serverURL + "/api/v1/documents")
	if err != nil {
		fmt.Printf("  读取文档列表失败：%v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		fmt.Printf("  读取文档列表失败：%s\n", strings.TrimSpace(string(b)))
		return
	}
	var lr listResp
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		fmt.Printf("  解析文档列表失败：%v\n", err)
		return
	}
	docsCache = lr.Documents
}

func upload(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("  打不开文件：%v\n", err)
		return
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		fmt.Printf("  构造请求失败：%v\n", err)
		return
	}
	if _, err := io.Copy(part, f); err != nil {
		fmt.Printf("  读取文件失败：%v\n", err)
		return
	}
	mw.Close()

	st, _ := f.Stat()
	fmt.Printf("  上传 %s（%.1f KB），分块嵌入中，可能要十几秒…\n",
		filepath.Base(path), float64(st.Size())/1024)

	start := time.Now()
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/documents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  上传失败：%v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	elapsed := time.Since(start).Round(time.Millisecond)

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("  上传失败（HTTP %d）：%s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	var out struct {
		DocumentID string `json:"document_id"`
		Filename   string `json:"filename"`
	}
	json.Unmarshal(body, &out)
	fmt.Printf("  ✓ 已入库 %s，耗时 %s\n", out.Filename, elapsed)
	refreshDocs()

	// 同名文件会重复入库，检索时新旧版本一起被召回，这里提醒一下
	same := 0
	for _, d := range docsCache {
		if d.Filename == out.Filename {
			same++
		}
	}
	if same > 1 {
		fmt.Printf("  提示：库里有 %d 个同名文件，旧版本没被覆盖。用 docs 看列表、rm 序号删掉旧的。\n", same)
	}
}

func deleteDoc(arg string) {
	if arg == "" {
		fmt.Println("  用法：rm <序号>   （序号见 docs 输出）")
		return
	}
	idx, err := strconv.Atoi(arg)
	if err != nil || idx < 1 || idx > len(docsCache) {
		fmt.Printf("  序号无效，先用 docs 看列表\n")
		return
	}
	d := docsCache[idx-1]

	req, _ := http.NewRequest(http.MethodDelete, serverURL+"/api/v1/documents?id="+d.ID, nil)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  删除失败：%v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		fmt.Printf("  删除失败（HTTP %d）：%s\n", resp.StatusCode, strings.TrimSpace(string(b)))
		return
	}
	fmt.Printf("  ✓ 已删除 %s（含 %d 个片段）\n", d.Filename, d.ChunkCount)
	refreshDocs()
}

func ask(question string) {
	payload, _ := json.Marshal(map[string]any{"question": question, "top_k": topK})

	start := time.Now()
	resp, err := client.Post(serverURL+"/api/v1/rag", "application/json", bytes.NewReader(payload))
	if err != nil {
		fmt.Printf("  提问失败：%v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	elapsed := time.Since(start).Round(time.Millisecond)

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("  提问失败（HTTP %d）：%s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	var out askResp
	if err := json.Unmarshal(body, &out); err != nil {
		fmt.Printf("  解析响应失败：%v\n", err)
		return
	}

	fmt.Println()
	printWrapped(out.Answer)
	fmt.Println()
	fmt.Printf("来源 %d 条，耗时 %s\n", len(out.Sources), elapsed)
	for i, s := range out.Sources {
		name := s.Filename
		if name == "" {
			name = "（未知文档）"
		}
		fmt.Printf("  [%d] %s  距离 %.3f\n", i+1, name, s.Distance)
		snippet := strings.ReplaceAll(strings.ReplaceAll(s.Content, "\n", " "), "\t", " ")
		if r := []rune(snippet); len(r) > 70 {
			snippet = string(r[:70]) + "…"
		}
		fmt.Printf("      %s\n", strings.TrimSpace(snippet))
	}
}

func setTopK(arg string) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 || n > 20 {
		fmt.Println("  用法：topk 5   （1-20）")
		return
	}
	topK = n
	fmt.Printf("  召回数已设为 %d\n", topK)
}

// ---------- 输出 ----------

func printBanner() {
	fmt.Println("RAG 测试终端  ——  直接打字提问，把文件拖进来就是上传")
	fmt.Printf("服务：%s\n", serverURL)
}

func printHelp() {
	fmt.Print(`
  怎么用
    打字          提问，例如：有哪些接口
    拖文件进来    上传并入库（也可以直接粘完整路径）
    docs          看已入库的文件
    rm 2          删掉列表里第 2 个文件
    topk 5        改每次召回的片段数（默认 3）
    exit          退出

  说明
    · 只支持 txt / md，文件需 UTF-8 编码
    · 上传耗时跟片段数成正比，约 0.3 秒一块
    · 这版服务重启不会清库了，之前入库的文档还在
`)
}

func printDocs() {
	if len(docsCache) == 0 {
		fmt.Println("  知识库是空的，拖一个文件进来试试。")
		return
	}
	fmt.Printf("  共 %d 个文件：\n", len(docsCache))
	for i, d := range docsCache {
		fmt.Printf("  %2d. %-28s %3d 片段  %6.1f KB\n",
			i+1, d.Filename, d.ChunkCount, float64(d.FileSize)/1024)
	}
}

// printWrapped 按终端宽度折行，避免长段落挤成一行
func printWrapped(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		fmt.Println("（模型返回了空内容）")
		return
	}
	limit := 76
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimRight(para, " ")
		if para == "" {
			fmt.Println()
			continue
		}
		var line []rune
		width := 0
		for _, r := range []rune(para) {
			w := 1
			if r > 0x2E80 { // 中日韩字符按两格宽算
				w = 2
			}
			if width+w > limit {
				fmt.Println(string(line))
				line = line[:0]
				width = 0
			}
			line = append(line, r)
			width += w
		}
		if len(line) > 0 {
			fmt.Println(string(line))
		}
	}
}
