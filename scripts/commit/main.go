package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

type fileChange struct {
	code    byte
	path    string
	oldPath string
}

type changeState struct {
	added   bool
	removed bool
}

func main() {
	changes, err := stagedChanges()
	if err != nil {
		fmt.Fprintf(os.Stderr, "commit: %v\n", err)
		os.Exit(1)
	}
	if len(changes) == 0 {
		fmt.Fprintln(os.Stderr, "commit: nothing staged")
		os.Exit(1)
	}

	subject := "update: " + time.Now().Format("02/01/06")
	body := describe(changes)
	printCommand(subject, body)
}

func stagedChanges() ([]fileChange, error) {
	out, err := exec.Command("git", "diff", "--cached", "--name-status", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("read staged changes: %w", err)
	}

	fields := strings.Split(string(out), "\x00")
	var changes []fileChange
	for i := 0; i < len(fields); {
		status := fields[i]
		if status == "" {
			i++
			continue
		}

		code := status[0]
		switch code {
		case 'R', 'C':
			if i+2 >= len(fields) {
				return nil, fmt.Errorf("malformed staged change")
			}
			changes = append(changes, fileChange{code: code, path: fields[i+2], oldPath: fields[i+1]})
			i += 3
		default:
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("malformed staged change")
			}
			changes = append(changes, fileChange{code: code, path: fields[i+1]})
			i += 2
		}
	}
	return changes, nil
}

func describe(changes []fileChange) []string {
	problems := make(map[string]*changeState)
	other := make(map[string]*changeState)

	for _, change := range changes {
		if slug, ok := problemSlug(change.path); ok {
			applyChange(stateFor(problems, slug), change.code)
		} else if label := otherLabel(change.path); label != "" {
			applyChange(stateFor(other, label), change.code)
		}

		if change.oldPath != "" && change.code == 'R' {
			if slug, ok := problemSlug(change.oldPath); ok {
				stateFor(problems, slug).removed = true
			} else if label := otherLabel(change.oldPath); label != "" {
				stateFor(other, label).removed = true
			}
		}
	}

	var solutionLines []string
	for slug, state := range problems {
		title := problemTitle(slug)
		solutionLines = append(solutionLines, describeState(state, title+" solution"))
	}
	sort.Strings(solutionLines)

	var otherLines []string
	for label, state := range other {
		otherLines = append(otherLines, describeState(state, label))
	}
	sort.Strings(otherLines)

	return append(solutionLines, otherLines...)
}

func stateFor(states map[string]*changeState, key string) *changeState {
	state, ok := states[key]
	if !ok {
		state = &changeState{}
		states[key] = state
	}
	return state
}

func applyChange(state *changeState, code byte) {
	switch code {
	case 'A', 'C', 'R':
		state.added = true
	case 'D':
		state.removed = true
	}
}

func describeState(state *changeState, label string) string {
	switch {
	case state.added && !state.removed:
		return "Added " + label
	case state.removed && !state.added:
		return "Remove " + label
	default:
		return "Updated " + label
	}
}

var problemPrefix = regexp.MustCompile(`^problemset/([^/]+)/`)

func problemSlug(path string) (string, bool) {
	match := problemPrefix.FindStringSubmatch(path)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func otherLabel(path string) string {
	switch {
	case path == "README.md":
		return "README"
	case path == "Makefile":
		return "Makefile"
	case path == "go.mod" || path == "go.sum":
		return "Go module"
	case path == "LICENSE":
		return "LICENSE"
	case path == ".gitignore":
		return ".gitignore"
	case strings.HasPrefix(path, "scripts/"):
		return "TUI scripts"
	default:
		if i := strings.IndexByte(path, '/'); i > 0 {
			return path[:i]
		}
		return path
	}
}

var leadingID = regexp.MustCompile(`^\d+-`)

var romanNumerals = map[string]bool{
	"i": true, "ii": true, "iii": true, "iv": true, "v": true,
	"vi": true, "vii": true, "viii": true, "ix": true, "x": true,
}

var smallWords = map[string]bool{
	"a": true, "an": true, "and": true, "as": true, "at": true,
	"but": true, "by": true, "for": true, "from": true, "in": true,
	"into": true, "nor": true, "of": true, "on": true, "onto": true,
	"or": true, "over": true, "per": true, "the": true, "to": true,
	"up": true, "via": true, "with": true,
}

func problemTitle(slug string) string {
	slug = leadingID.ReplaceAllString(slug, "")
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		switch {
		case romanNumerals[lower]:
			parts[i] = strings.ToUpper(part)
		case i > 0 && i < len(parts)-1 && smallWords[lower]:
			parts[i] = lower
		default:
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func printCommand(subject string, body []string) {
	fmt.Printf("git commit -m %s", shellQuote(subject))
	if len(body) > 0 {
		fmt.Printf(" -m %s", shellQuote(strings.Join(body, "\n")))
	}
	fmt.Println()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
