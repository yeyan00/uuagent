package agent_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func TestRealUserDocxSkillCanGenerateDocxThroughUUAgentTools(t *testing.T) {
	skillRoot := filepath.Join(os.Getenv("USERPROFILE"), ".uuagent", "skills", "docx")
	if _, err := os.Stat(filepath.Join(skillRoot, "SKILL.md")); err != nil {
		t.Skipf("real user docx skill not installed at %s: %v", skillRoot, err)
	}
	testHome := filepath.Join(t.TempDir(), "home")
	testSkillRoot := filepath.Join(testHome, "skills", "docx")
	if err := os.MkdirAll(testSkillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SKILL.md", "docx-js.md"} {
		data, err := os.ReadFile(filepath.Join(skillRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(testSkillRoot, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("UUAGENT_HOME", testHome)
	t.Setenv("UUAGENT_PROXY_URL", "")
	t.Setenv("UUAGENT_MODEL", "")

	calls := 0
	var firstSystemPrompt string
	var shellResult string
	resourcePath := strings.ReplaceAll(filepath.Join(testSkillRoot, "docx-js.md"), `\`, `\\`)
	createScript := strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"$resource = Get-Content -LiteralPath '" + resourcePath + "' -Raw",
		"if ($resource -notmatch 'Document') { throw 'docx-js.md was not readable' }",
		"Remove-Item -LiteralPath 'test.docx' -Force -ErrorAction SilentlyContinue",
		"Add-Type -AssemblyName System.IO.Compression",
		"Add-Type -AssemblyName System.IO.Compression.FileSystem",
		"$zip = [System.IO.Compression.ZipFile]::Open('test.docx', [System.IO.Compression.ZipArchiveMode]::Create)",
		"$utf8 = [System.Text.Encoding]::UTF8",
		"function Add-ZipText($name, $text) { $entry = $zip.CreateEntry($name); $stream = $entry.Open(); $writer = New-Object System.IO.StreamWriter($stream, $utf8); $writer.Write($text); $writer.Dispose(); $stream.Dispose() }",
		"Add-ZipText '[Content_Types].xml' '<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?><Types xmlns=\"http://schemas.openxmlformats.org/package/2006/content-types\"><Default Extension=\"rels\" ContentType=\"application/vnd.openxmlformats-package.relationships+xml\"/><Default Extension=\"xml\" ContentType=\"application/xml\"/><Override PartName=\"/word/document.xml\" ContentType=\"application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml\"/></Types>'",
		"Add-ZipText '_rels/.rels' '<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?><Relationships xmlns=\"http://schemas.openxmlformats.org/package/2006/relationships\"><Relationship Id=\"rId1\" Type=\"http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument\" Target=\"word/document.xml\"/></Relationships>'",
		"Add-ZipText 'word/document.xml' '<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?><w:document xmlns:w=\"http://schemas.openxmlformats.org/wordprocessingml/2006/main\"><w:body><w:p><w:r><w:t>UUAgent real docx skill generated test.docx</w:t></w:r></w:p><w:sectPr/></w:body></w:document>'",
		"$zip.Dispose()",
		"'created test.docx using resource length ' + $resource.Length",
	}, "; ")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			var req struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode first request: %v", err)
			}
			if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
				firstSystemPrompt = req.Messages[0].Content
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-shell", "type": "function", "function": map[string]any{"name": "shell", "arguments": mustJSON(map[string]string{"command": createScript})}}}}}}})
			return
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, msg := range req.Messages {
			if msg.Role == "tool" {
				shellResult = msg.Content
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	a := agent.New(cfg)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(cwd, "test.docx")
	_ = os.Remove(docPath)
	t.Cleanup(func() {
		_ = os.Remove(docPath)
	})
	events, err := a.Run(context.Background(), "real-docx-skill", "/skill:docx create test.docx")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatalf("unexpected agent error: %s", evt.Text)
		}
	}
	if !strings.Contains(firstSystemPrompt, "DOCX creation") || !strings.Contains(firstSystemPrompt, "docx-js.md") {
		t.Fatalf("/skill:docx did not inject real SKILL.md content: %q", firstSystemPrompt)
	}
	if !strings.Contains(shellResult, "created test.docx using resource length") {
		t.Fatalf("shell tool did not read docx skill resource and create docx: %s", shellResult)
	}
	if _, err := os.Stat(docPath); err != nil {
		t.Fatalf("test.docx was not created in agent tool working directory: %v", err)
	}
	zr, err := zip.OpenReader(docPath)
	if err != nil {
		t.Fatalf("test.docx is not a valid zip/docx: %v", err)
	}
	defer zr.Close()
	foundDocumentXML := false
	var entries []string
	for _, f := range zr.File {
		entries = append(entries, f.Name)
		if f.Name == "word/document.xml" {
			foundDocumentXML = true
			break
		}
	}
	if !foundDocumentXML {
		t.Fatalf("test.docx missing word/document.xml; entries=%v", entries)
	}
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
