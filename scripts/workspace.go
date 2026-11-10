package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	problemsDir  = "problemset"
	tableHeader  = "| # | Title | Solution | Difficulty |"
	tableDivider = "|:---:|---|:---:|:---:|"
)

type scaffoldResult struct {
	Path          string
	Created       bool
	ReadmeUpdated bool
}

func locateRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if root := strings.TrimSpace(string(out)); root != "" {
			return root, nil
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for _, marker := range []string{problemsDir, "README.md"} {
		if _, statErr := os.Stat(filepath.Join(cwd, marker)); statErr == nil {
			return cwd, nil
		}
	}
	return "", errors.New("not inside a git repository")
}

func scaffold(ctx context.Context, root string, p *problem, lang language) (scaffoldResult, error) {
	dir := filepath.Join(root, problemsDir, p.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return scaffoldResult{}, fmt.Errorf("create problem folder: %w", err)
	}

	path := filepath.Join(dir, "main."+lang.Ext)
	created := false
	switch info, err := os.Stat(path); {
	case err == nil && info.IsDir():
		return scaffoldResult{}, fmt.Errorf("%s is a directory", path)
	case err == nil:
		// Keep an existing solution.
	case errors.Is(err, fs.ErrNotExist):
		if err := os.WriteFile(path, []byte(p.snippet(lang)), 0o644); err != nil {
			return scaffoldResult{}, fmt.Errorf("write solution file: %w", err)
		}
		created = true
	default:
		return scaffoldResult{}, fmt.Errorf("inspect solution file: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return scaffoldResult{}, err
	}

	updated, err := updateReadme(root, p)
	if err != nil {
		return scaffoldResult{}, err
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	return scaffoldResult{Path: filepath.ToSlash(rel), Created: created, ReadmeUpdated: updated}, nil
}

type readmeRow struct {
	ID           int
	IDText       string
	TitleCell    string
	SolutionCell string
	Difficulty   string
	Slug         string
}

var (
	leetcodeSlugRE = regexp.MustCompile(`leetcode\.(?:com|cn)/problems/([A-Za-z0-9_-]+)`)
	localSlugRE    = regexp.MustCompile(`problemset/([A-Za-z0-9_-]+)/`)
)

func updateReadme(root string, p *problem) (bool, error) {
	path := filepath.Join(root, "README.md")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("read README.md: %w", err)
	}
	original := string(data)
	content := original

	var rows []readmeRow
	lines := strings.Split(content, "\n")
	if info, ok := findTable(lines); ok {
		rows = parseRows(strings.Join(lines[info.rowStart:info.rowEnd], "\n"))
		rows = upsertRow(rows, p)
		rows = refreshSolutions(root, rows)
		content = strings.Join(replaceTableLines(lines, info, rows), "\n")
	} else {
		rows = upsertRow(nil, p)
		rows = refreshSolutions(root, rows)
		content = strings.TrimRight(content, "\n")
		if content == "" {
			content = "# Leetcode"
		}
		content += "\n\n## Problems\n\n" + renderSection(rows)
	}

	content = centerTitle(content)
	content = updateStats(content, root, rows)
	content = strings.TrimRight(content, "\n") + "\n"
	if content == original {
		return false, nil
	}
	if err := writeFileAtomic(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write README.md: %w", err)
	}
	return true, nil
}

func centerTitle(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			lines[i] = `<h1 align="center">` + title + `</h1>`
		}
		break
	}
	return strings.Join(lines, "\n")
}

func updateStats(content, root string, rows []readmeRow) string {
	block := renderStats(root, rows)
	lines := strings.Split(content, "\n")
	if start, end, ok := findStatsBlock(lines); ok {
		out := make([]string, 0, len(lines))
		out = append(out, lines[:start]...)
		out = append(out, strings.Split(block, "\n")...)
		out = append(out, lines[end+1:]...)
		return strings.Join(out, "\n")
	}
	return insertStats(content, block)
}

func findStatsBlock(lines []string) (int, int, bool) {
	for i, line := range lines {
		if !strings.Contains(line, "img.shields.io/badge/Solved-") {
			continue
		}
		start := -1
		for j := i; j >= 0; j-- {
			if strings.TrimSpace(lines[j]) == `<div align="center">` {
				start = j
				break
			}
		}
		if start < 0 {
			continue
		}
		for k := i; k < len(lines); k++ {
			if strings.TrimSpace(lines[k]) == "</div>" {
				return start, k, true
			}
		}
	}
	return 0, 0, false
}

func insertStats(content, block string) string {
	lines := strings.Split(content, "\n")
	titleIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			titleIndex = i
			break
		}
	}
	if titleIndex < 0 {
		return block + "\n"
	}

	rest := lines[titleIndex+1:]
	for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}

	out := make([]string, 0, len(lines)+4)
	out = append(out, lines[:titleIndex+1]...)
	out = append(out, "", block)
	if len(rest) > 0 {
		out = append(out, "")
		out = append(out, rest...)
	}
	return strings.Join(out, "\n")
}

func renderStats(root string, rows []readmeRow) string {
	easy, medium, hard := 0, 0, 0
	languageCounts := make(map[string]int)

	for _, row := range rows {
		difficulty := strings.ToLower(strings.TrimSpace(row.Difficulty))
		switch {
		case strings.Contains(difficulty, "easy"):
			easy++
		case strings.Contains(difficulty, "medium"):
			medium++
		case strings.Contains(difficulty, "hard"):
			hard++
		}

		if row.Slug == "" {
			continue
		}
		for _, lang := range languages {
			path := filepath.Join(root, problemsDir, row.Slug, "main."+lang.Ext)
			if fileExists(path) {
				languageCounts[lang.Name]++
			}
		}
	}

	var b strings.Builder
	b.WriteString("<div align=\"center\">\n\n")
	fmt.Fprintf(&b, "%s %s %s %s\n",
		statsBadge("Solved", len(rows), "blue"),
		statsBadge("Easy", easy, "brightgreen"),
		statsBadge("Medium", medium, "yellow"),
		statsBadge("Hard", hard, "red"))

	var languageBadges []string
	for _, lang := range languages {
		if count := languageCounts[lang.Name]; count > 0 {
			languageBadges = append(languageBadges, statsBadge(lang.Name, count, languageColor(lang.Name)))
		}
	}
	if len(languageBadges) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(languageBadges, " "))
		b.WriteString("\n")
	}

	b.WriteString("\n</div>")
	return b.String()
}

func statsBadge(label string, count int, color string) string {
	escaped := strings.ReplaceAll(label, " ", "_")
	escaped = strings.ReplaceAll(escaped, "+", "%2B")
	return fmt.Sprintf("![%s](https://img.shields.io/badge/%s-%d-%s)", label, escaped, count, color)
}

func languageColor(name string) string {
	switch name {
	case "Python":
		return "3776AB"
	case "Go":
		return "00ADD8"
	case "C++":
		return "00599C"
	default:
		return "blue"
	}
}

type tableInfo struct {
	headerIndex int
	rowStart    int
	rowEnd      int
	header      string
	divider     string
}

func findTable(lines []string) (tableInfo, bool) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") ||
			!strings.Contains(strings.ToLower(trimmed), "problem") {
			continue
		}
		if info, ok := findTableAfter(lines, i+1); ok {
			return info, true
		}
	}
	return findTableAfter(lines, 0)
}

func findTableAfter(lines []string, start int) (tableInfo, bool) {
	for i := start; i+1 < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitRow(line)
		if len(cells) < 4 ||
			!strings.EqualFold(cells[0], "#") ||
			!strings.Contains(strings.ToLower(cells[1]), "title") ||
			!strings.Contains(strings.ToLower(cells[2]), "solution") {
			continue
		}
		if !isDividerLine(lines[i+1]) {
			continue
		}
		end := i + 2
		for end < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end]), "|") {
			end++
		}
		return tableInfo{
			headerIndex: i,
			rowStart:    i + 2,
			rowEnd:      end,
			header:      lines[i],
			divider:     lines[i+1],
		}, true
	}
	return tableInfo{}, false
}

func replaceTableLines(lines []string, info tableInfo, rows []readmeRow) []string {
	header := info.header
	divider := info.divider
	if len(splitRow(header)) != 4 || len(splitRow(divider)) != 4 {
		header = tableHeader
		divider = tableDivider
	}

	out := make([]string, 0, len(lines)+len(rows))
	out = append(out, lines[:info.headerIndex]...)
	out = append(out, header, divider)
	out = append(out, renderTableRows(rows)...)
	out = append(out, lines[info.rowEnd:]...)
	return out
}

func isDividerLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.Contains(trimmed, "-") {
		return false
	}
	return isDivider(splitRow(trimmed))
}

func parseRows(section string) []readmeRow {
	var rows []readmeRow
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitRow(line)
		if len(cells) < 4 || strings.EqualFold(cells[0], "#") || isDivider(cells) {
			continue
		}
		row := readmeRow{
			IDText:       cells[0],
			TitleCell:    cells[1],
			SolutionCell: cells[2],
			Difficulty:   cells[3],
		}
		row.ID, _ = strconv.Atoi(row.IDText)
		row.Slug = slugFromCell(row.TitleCell)
		if row.Slug == "" {
			row.Slug = slugFromCell(row.SolutionCell)
		}
		if row.IDText == "" && row.TitleCell == "" {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")

	var cells []string
	var current strings.Builder
	escaped := false
	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if r == '|' {
			cells = append(cells, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	cells = append(cells, strings.TrimSpace(current.String()))
	return cells
}

func isDivider(cells []string) bool {
	for _, cell := range cells {
		if strings.Trim(cell, ":- ") != "" {
			return false
		}
	}
	return true
}

func slugFromCell(cell string) string {
	if match := leetcodeSlugRE.FindStringSubmatch(cell); len(match) == 2 {
		return strings.ToLower(match[1])
	}
	if match := localSlugRE.FindStringSubmatch(cell); len(match) == 2 {
		return strings.ToLower(match[1])
	}
	return ""
}

func upsertRow(rows []readmeRow, p *problem) []readmeRow {
	id, _ := strconv.Atoi(p.ID)
	for i := range rows {
		if rows[i].Slug == p.Slug || (id > 0 && rows[i].ID == id) {
			rows[i].ID = id
			rows[i].IDText = p.ID
			rows[i].TitleCell = titleCell(p)
			rows[i].Difficulty = difficultyCell(p.Difficulty)
			rows[i].Slug = p.Slug
			return rows
		}
	}
	return append(rows, readmeRow{
		ID:         id,
		IDText:     p.ID,
		TitleCell:  titleCell(p),
		Difficulty: difficultyCell(p.Difficulty),
		Slug:       p.Slug,
	})
}

func refreshSolutions(root string, rows []readmeRow) []readmeRow {
	for i := range rows {
		if rows[i].Slug == "" {
			continue
		}
		if cell := solutionCell(root, rows[i].Slug); cell != "" {
			rows[i].SolutionCell = cell
		}
	}
	return rows
}

func solutionCell(root, slug string) string {
	var links []string
	for _, lang := range languages {
		rel := filepath.Join(problemsDir, slug, "main."+lang.Ext)
		if fileExists(filepath.Join(root, rel)) {
			links = append(links, fmt.Sprintf("[%s](./%s)", lang.Name, filepath.ToSlash(rel)))
		}
	}
	return strings.Join(links, ", ")
}

func renderSection(rows []readmeRow) string {
	lines := []string{tableHeader, tableDivider}
	lines = append(lines, renderTableRows(rows)...)
	return strings.Join(lines, "\n")
}

func renderTableRows(rows []readmeRow) []string {
	sortRows(rows)

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, fmt.Sprintf("| %s | %s | %s | %s |",
			idLabel(row), row.TitleCell, row.SolutionCell, row.Difficulty))
	}
	return out
}

func idLabel(row readmeRow) string {
	if row.ID > 0 {
		return fmt.Sprintf("%04d", row.ID)
	}
	if row.IDText != "" {
		return row.IDText
	}
	return "-"
}

func titleCell(p *problem) string {
	title := strings.ReplaceAll(p.Title, "|", "\\|")
	title = strings.ReplaceAll(title, "[", "\\[")
	title = strings.ReplaceAll(title, "]", "\\]")
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")
	title = strings.ReplaceAll(title, "\t", " ")
	return fmt.Sprintf("[%s](https://leetcode.com/problems/%s)", title, p.Slug)
}

func difficultyCell(difficulty string) string {
	if strings.TrimSpace(difficulty) == "" {
		return "-"
	}
	return difficulty
}

func sortRows(rows []readmeRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch {
		case a.ID == 0 && b.ID == 0:
			return a.TitleCell < b.TitleCell
		case a.ID == 0:
			return false
		case b.ID == 0:
			return true
		case a.ID != b.ID:
			return a.ID < b.ID
		default:
			return a.TitleCell < b.TitleCell
		}
	})
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func writeFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	tmpName = ""
	return nil
}
