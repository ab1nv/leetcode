package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const graphQLQuery = `query questionData($titleSlug: String!) {
	question(titleSlug: $titleSlug) {
		questionFrontendId
		questionTitle
		titleSlug
		difficulty
		codeSnippets { langSlug code }
	}
}`

type language struct {
	Name     string
	Ext      string
	LangSlug string
}

var languages = []language{
	{Name: "Python", Ext: "py", LangSlug: "python3"},
	{Name: "Go", Ext: "go", LangSlug: "golang"},
	{Name: "C++", Ext: "cpp", LangSlug: "cpp"},
}

type problem struct {
	ID         string
	Title      string
	Slug       string
	Difficulty string
	snippets   map[string]string
}

func (p *problem) snippet(lang language) string {
	if code, ok := p.snippets[lang.LangSlug]; ok {
		return code
	}
	if lang.Ext == "py" {
		if code, ok := p.snippets["python"]; ok {
			return code
		}
	}
	return fallbackSnippet(lang)
}

func fallbackSnippet(lang language) string {
	switch lang.Ext {
	case "py":
		return "class Solution:\n    pass\n"
	case "go":
		return "func solution() {\n}\n"
	case "cpp":
		return "class Solution {\npublic:\n};\n"
	default:
		return ""
	}
}

type leetcodeClient struct {
	http *http.Client
}

func newLeetcodeClient() *leetcodeClient {
	return &leetcodeClient{http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *leetcodeClient) fetchProblem(ctx context.Context, slug string) (*problem, error) {
	body, err := json.Marshal(map[string]any{
		"operationName": "questionData",
		"variables":     map[string]string{"titleSlug": slug},
		"query":         graphQLQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://leetcode.com/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://leetcode.com/problems/"+slug)
	req.Header.Set("User-Agent", "leetcode-tui/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request leetcode: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("leetcode returned HTTP %d", resp.StatusCode)
	}

	var wrapper struct {
		Data struct {
			Question *struct {
				ID         string `json:"questionFrontendId"`
				Title      string `json:"questionTitle"`
				Slug       string `json:"titleSlug"`
				Difficulty string `json:"difficulty"`
				Snippets   []struct {
					LangSlug string `json:"langSlug"`
					Code     string `json:"code"`
				} `json:"codeSnippets"`
			} `json:"question"`
		} `json:"data"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	q := wrapper.Data.Question
	if q == nil {
		return nil, fmt.Errorf("problem %q not found", slug)
	}

	p := &problem{
		ID:         q.ID,
		Title:      q.Title,
		Slug:       q.Slug,
		Difficulty: q.Difficulty,
		snippets:   make(map[string]string, len(q.Snippets)),
	}
	if p.Slug == "" || !slugPattern.MatchString(p.Slug) {
		p.Slug = slug
	}
	if p.Title == "" {
		p.Title = p.Slug
	}
	for _, snippet := range q.Snippets {
		p.snippets[snippet.LangSlug] = snippet.Code
	}
	return p, nil
}

var (
	slugPattern    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	problemsPathRE = regexp.MustCompile(`/problems/([A-Za-z0-9_-]+)`)
)

func parseProblemSlug(raw string) (string, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return "", errors.New("enter a question link")
	}

	if !strings.ContainsAny(input, "/:") {
		slug := strings.ToLower(input)
		if slugPattern.MatchString(slug) {
			return slug, nil
		}
		return "", fmt.Errorf("invalid problem slug %q", input)
	}

	candidate := input
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("invalid link: %w", err)
	}

	switch strings.ToLower(parsed.Hostname()) {
	case "leetcode.com", "www.leetcode.com", "leetcode.cn", "www.leetcode.cn":
	default:
		return "", errors.New("only LeetCode links are supported")
	}

	match := problemsPathRE.FindStringSubmatch(parsed.Path)
	if len(match) != 2 {
		return "", errors.New("link does not contain /problems/<slug>")
	}

	slug := strings.ToLower(match[1])
	if !slugPattern.MatchString(slug) {
		return "", fmt.Errorf("invalid problem slug %q", slug)
	}
	return slug, nil
}
