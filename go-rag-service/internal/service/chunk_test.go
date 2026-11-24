package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChunkTextMarkdownHeadingMergedWithBody(t *testing.T) {
	text := "## 项目概述\n\n这是一个 RAG 服务。\n\n## 接口\n\n- 上传\n- 问答\n"
	chunks := chunkText(text, 500)

	if len(chunks) != 2 {
		t.Fatalf("期望 2 块，实际 %d 块: %#v", len(chunks), chunks)
	}
	if !strings.HasPrefix(chunks[0], "## 项目概述") || !strings.Contains(chunks[0], "这是一个 RAG 服务。") {
		t.Errorf("第 1 块应包含标题和正文，实际: %q", chunks[0])
	}
	if !strings.Contains(chunks[1], "上传") || !strings.Contains(chunks[1], "问答") {
		t.Errorf("第 2 块应包含列表项，实际: %q", chunks[1])
	}
}

func TestChunkTextIgnoresHashInsideCodeFence(t *testing.T) {
	text := "## 快速启动\n\n```bash\n# 1. 启动数据库\n# 2. 修改 api_key\n./run.sh\n```\n\n后续说明。\n"
	chunks := chunkText(text, 500)

	if len(chunks) != 1 {
		t.Fatalf("代码块内的 # 不应触发切分，期望 1 块，实际 %d 块: %#v", len(chunks), chunks)
	}
	for _, c := range chunks {
		if strings.HasPrefix(c, "# 1.") {
			t.Errorf("不应产生以 shell 注释开头的碎片: %q", c)
		}
	}
	if !strings.Contains(chunks[0], "后续说明") {
		t.Errorf("正文应与标题同块，实际: %q", chunks[0])
	}
}

func TestChunkTextPlainTextSplitsOnBlankLine(t *testing.T) {
	text := "第一段内容。\n\n第二段内容。\n"
	chunks := chunkText(text, 500)

	if len(chunks) != 2 {
		t.Fatalf("纯文本应按空行分段，期望 2 块，实际 %d 块: %#v", len(chunks), chunks)
	}
}

func TestChunkTextRespectsMaxChars(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("这是一行用于测试长度上限的正文内容。\n")
	}
	chunks := chunkText(sb.String(), 200)

	if len(chunks) < 2 {
		t.Fatalf("超过上限应切成多块，实际 %d 块", len(chunks))
	}
	for i, c := range chunks {
		if n := utf8.RuneCountInString(c); n > 200 {
			t.Errorf("第 %d 块 %d 字，超过上限 200", i, n)
		}
	}
}

func TestChunkTextLongSingleLineHardSplit(t *testing.T) {
	line := strings.Repeat("长", 600)
	chunks := chunkText(line, 200)

	if len(chunks) != 3 {
		t.Fatalf("600 字单行按 200 上限应切成 3 块，实际 %d 块", len(chunks))
	}
}

func TestChunkTextEmpty(t *testing.T) {
	if got := chunkText("   \n\n  ", 500); got != nil {
		t.Errorf("空白文本应返回 nil，实际 %#v", got)
	}
}

func TestChunkTextMergesHeadingOnlySection(t *testing.T) {
	text := "## 数据流\n\n### 文档摄入\n\n用户上传文件后自动分段。\n\n### 问答流程\n\n检索后生成回答。\n"
	chunks := chunkText(text, 500)

	for _, c := range chunks {
		if strings.TrimSpace(c) == "## 数据流" {
			t.Fatalf("不应存在只有父标题的零信息块: %#v", chunks)
		}
	}
	if len(chunks) != 2 {
		t.Fatalf("期望 2 块，实际 %d 块: %#v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0], "## 数据流") || !strings.Contains(chunks[0], "用户上传文件后自动分段") {
		t.Errorf("父标题应并入后续正文块，实际: %q", chunks[0])
	}
}

// 多个 ## 存在时应按 ## 切分，### 子节留在父块内，
// 否则「### 2. 上传文档」这类子节会变成孤立小块，检索时被漏掉。
func TestChunkTextKeepsSubsectionsInParent(t *testing.T) {
	text := "# 手册\n\n## API 接口\n\n### 1. 健康检查\n\nGET /health\n\n" +
		"### 2. 上传文档\n\nPOST /api/v1/documents\n\n### 3. RAG 问答\n\nPOST /api/v1/rag\n\n" +
		"## 已知限制\n\n仅支持 txt 和 md。\n"
	chunks := chunkText(text, 500)

	var apiChunk string
	for _, c := range chunks {
		if strings.Contains(c, "## API 接口") {
			apiChunk = c
			break
		}
	}
	if apiChunk == "" {
		t.Fatalf("未找到 API 接口块: %#v", chunks)
	}
	for _, want := range []string{"健康检查", "上传文档", "RAG 问答"} {
		if !strings.Contains(apiChunk, want) {
			t.Errorf("API 接口块应包含全部子节 %q，实际: %q", want, apiChunk)
		}
	}
	if len(chunks) != 2 {
		t.Errorf("期望 2 块（标题并入 API 块），实际 %d 块: %#v", len(chunks), chunks)
	}
}
