//go:build ignore

// Count effective source lines in this repository.
//
// Usage:
//   go run scripts/count_loc.go
//   go run scripts/count_loc.go -all        # include test files
//   go run scripts/count_loc.go -by-file    # print every counted file
//
// Effective lines are non-blank, non-comment lines. Vendor/dependency/build
// directories are excluded by default.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fileStat struct {
	Path  string
	Ext   string
	Lines int
}

type lang struct {
	LineComments  []string
	BlockComments [][2]string
}

var langs = map[string]lang{
	".go":   {LineComments: []string{"//"}, BlockComments: [][2]string{{"/*", "*/"}}},
	".ts":   {LineComments: []string{"//"}, BlockComments: [][2]string{{"/*", "*/"}}},
	".tsx":  {LineComments: []string{"//"}, BlockComments: [][2]string{{"/*", "*/"}, {"{/*", "*/}"}}},
	".js":   {LineComments: []string{"//"}, BlockComments: [][2]string{{"/*", "*/"}}},
	".jsx":  {LineComments: []string{"//"}, BlockComments: [][2]string{{"/*", "*/"}, {"{/*", "*/}"}}},
	".css":  {BlockComments: [][2]string{{"/*", "*/"}}},
	".html": {BlockComments: [][2]string{{"<!--", "-->",}}},
	".sh":   {LineComments: []string{"#"}},
	".ps1":  {LineComments: []string{"#"}, BlockComments: [][2]string{{"<#", "#>"}}},
	".bat":  {LineComments: []string{"REM ", "rem ", "::"}},
}

var skipDirs = map[string]bool{
	".git": true, ".pi": true, ".tools": true, ".vscode": true,
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"coverage": true, ".next": true, ".vite": true,
}

func main() {
	includeTests := flag.Bool("all", false, "include test files")
	byFile := flag.Bool("by-file", false, "print per-file counts")
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	stats, err := collect(*root, *includeTests)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	sort.Slice(stats, func(i, j int) bool { return stats[i].Path < stats[j].Path })
	if *byFile {
		for _, s := range stats {
			fmt.Printf("%6d  %s\n", s.Lines, filepath.ToSlash(s.Path))
		}
		fmt.Println()
	}

	totals := map[string]int{}
	files := map[string]int{}
	totalLines := 0
	for _, s := range stats {
		totals[s.Ext] += s.Lines
		files[s.Ext]++
		totalLines += s.Lines
	}

	exts := make([]string, 0, len(totals))
	for ext := range totals {
		exts = append(exts, ext)
	}
	sort.Slice(exts, func(i, j int) bool {
		if totals[exts[i]] == totals[exts[j]] {
			return exts[i] < exts[j]
		}
		return totals[exts[i]] > totals[exts[j]]
	})

	fmt.Printf("Effective source lines: %d\n", totalLines)
	fmt.Printf("Files counted: %d\n", len(stats))
	fmt.Println("\nBy extension:")
	for _, ext := range exts {
		fmt.Printf("  %-5s %4d files  %6d lines\n", ext, files[ext], totals[ext])
	}
}

func collect(root string, includeTests bool) ([]fileStat, error) {
	var stats []fileStat
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		if _, ok := langs[ext]; !ok {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "scripts/count_loc.go" {
			return nil
		}
		if !includeTests && isTestFile(name) {
			return nil
		}
		if isGenerated(path) {
			return nil
		}

		lines, err := countFile(path, langs[ext])
		if err != nil {
			return err
		}
		stats = append(stats, fileStat{Path: rel, Ext: ext, Lines: lines})
		return nil
	})
	return stats, err
}

func isTestFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, "_test.go") ||
		strings.Contains(lower, ".test.") ||
		strings.Contains(lower, ".spec.")
}

func isGenerated(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for i := 0; i < 20; i++ {
		line, err := r.ReadString('\n')
		if strings.Contains(line, "Code generated") || strings.Contains(line, "DO NOT EDIT") {
			return true
		}
		if err != nil {
			break
		}
	}
	return false
}

func countFile(path string, l lang) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}

	inBlockEnd := ""
	count := 0
	s := bufio.NewScanner(strings.NewReader(string(data)))
	for s.Scan() {
		line := s.Text()
		if effectiveLine(line, l, &inBlockEnd) {
			count++
		}
	}
	return count, s.Err()
}

func effectiveLine(line string, l lang, inBlockEnd *string) bool {
	s := strings.TrimSpace(line)
	hasCode := false

	for s != "" {
		if *inBlockEnd != "" {
			idx := strings.Index(s, *inBlockEnd)
			if idx < 0 {
				return hasCode
			}
			s = strings.TrimSpace(s[idx+len(*inBlockEnd):])
			*inBlockEnd = ""
			continue
		}

		lineIdx, _ := firstLineComment(s, l.LineComments)
		blockIdx, blockEnd := firstBlockComment(s, l.BlockComments)

		if lineIdx >= 0 && (blockIdx < 0 || lineIdx < blockIdx) {
			if strings.TrimSpace(s[:lineIdx]) != "" {
				hasCode = true
			}
			return hasCode
		}
		if blockIdx >= 0 {
			if strings.TrimSpace(s[:blockIdx]) != "" {
				hasCode = true
			}
			afterStart := s[blockIdx+len(blockEnd[0]):]
			endIdx := strings.Index(afterStart, blockEnd[1])
			if endIdx < 0 {
				*inBlockEnd = blockEnd[1]
				return hasCode
			}
			s = strings.TrimSpace(afterStart[endIdx+len(blockEnd[1]):])
			continue
		}

		if s != "" {
			hasCode = true
		}
		return hasCode
	}
	return hasCode
}

func firstLineComment(s string, markers []string) (int, string) {
	best := -1
	marker := ""
	for _, m := range markers {
		idx := strings.Index(s, m)
		if idx >= 0 && (best < 0 || idx < best) {
			best = idx
			marker = m
		}
	}
	return best, marker
}

func firstBlockComment(s string, markers [][2]string) (int, [2]string) {
	best := -1
	var marker [2]string
	for _, m := range markers {
		idx := strings.Index(s, m[0])
		if idx >= 0 && (best < 0 || idx < best) {
			best = idx
			marker = m
		}
	}
	return best, marker
}
