package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type screen int

const (
	screenInput screen = iota
	screenLanguage
	screenWorking
	screenDone
)

type outcome struct {
	OK      bool
	Title   string
	Message string
	Details []string
}

type progressEvent struct {
	Title string
}

type doneEvent struct {
	Outcome outcome
}

type app struct {
	root   string
	term   *terminal
	theme  theme
	client *leetcodeClient

	screen   screen
	field    textField
	inputErr string
	slug     string
	lang     int

	workingTitle string
	spinner      int
	running      bool

	outcome outcome

	events chan any
	cancel context.CancelFunc
	quit   bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "leet: %v\n", err)
		os.Exit(1)
	}
}

func run() (err error) {
	root, err := locateRoot()
	if err != nil {
		return err
	}

	term, err := openTerminal()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := term.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	a := &app{
		root:   root,
		term:   term,
		theme:  newTheme(),
		client: newLeetcodeClient(),
		events: make(chan any, 16),
	}

	keys := make(chan keyEvent, 16)
	go readKeys(term.in, keys)

	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGWINCH)
	defer signal.Stop(signals)

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	render := true
	for !a.quit {
		select {
		case key, ok := <-keys:
			if !ok {
				a.quit = true
				continue
			}
			a.handleKey(key)
			render = true
		case event := <-a.events:
			a.handleEvent(event)
			render = true
		case <-ticker.C:
			if a.running {
				a.spinner++
				render = true
			}
		case sig := <-signals:
			if sig == syscall.SIGWINCH {
				render = true
				continue
			}
			a.quit = true
			a.cancelWork()
		}

		if render && !a.quit {
			if err := a.render(); err != nil {
				return err
			}
			render = false
		}
	}

	a.cancelWork()
	return nil
}

func (a *app) handleKey(key keyEvent) {
	if key.kind == keyCtrlC {
		a.quit = true
		a.cancelWork()
		return
	}

	switch a.screen {
	case screenInput:
		a.handleInputKey(key)
	case screenLanguage:
		a.handleLanguageKey(key)
	case screenWorking:
		// Ignore input while work is running.
	case screenDone:
		a.handleDoneKey(key)
	}
}

func (a *app) handleInputKey(key keyEvent) {
	switch key.kind {
	case keyEsc:
		a.quit = true
	case keyEnter:
		slug, err := parseProblemSlug(a.field.value())
		if err != nil {
			a.inputErr = err.Error()
			return
		}
		a.slug = slug
		a.inputErr = ""
		a.lang = 0
		a.screen = screenLanguage
	default:
		if a.field.handleKey(key) {
			a.inputErr = ""
		}
	}
}

func (a *app) handleLanguageKey(key keyEvent) {
	switch {
	case key.kind == keyEsc:
		a.screen = screenInput
	case key.kind == keyUp || isRune(key, 'k'):
		if a.lang > 0 {
			a.lang--
		}
	case key.kind == keyDown || isRune(key, 'j'):
		if a.lang < len(languages)-1 {
			a.lang++
		}
	case key.kind == keyHome:
		a.lang = 0
	case key.kind == keyEnd:
		a.lang = len(languages) - 1
	case key.kind == keyEnter:
		a.startWork()
	case key.kind == keyRune:
		if index, ok := digitIndex(key.text, len(languages)); ok {
			a.lang = index
			a.startWork()
		}
	}
}

func (a *app) handleDoneKey(_ keyEvent) {
	if a.outcome.OK {
		a.quit = true
		return
	}
	a.screen = screenInput
	a.inputErr = ""
}

func (a *app) startWork() {
	if a.lang < 0 || a.lang >= len(languages) {
		return
	}

	lang := languages[a.lang]
	slug := a.slug
	root := a.root
	client := a.client
	events := a.events

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.screen = screenWorking
	a.workingTitle = "Fetching problem"
	a.spinner = 0
	a.running = true
	a.outcome = outcome{}

	go func() {
		sendEvent(ctx, events, progressEvent{Title: "Fetching problem"})
		p, err := client.fetchProblem(ctx, slug)
		if err != nil {
			sendEvent(ctx, events, doneEvent{Outcome: outcome{
				Title:   "Could not fetch problem",
				Message: err.Error(),
			}})
			return
		}

		sendEvent(ctx, events, progressEvent{Title: "Creating files"})
		result, err := scaffold(ctx, root, p, lang)
		if err != nil {
			sendEvent(ctx, events, doneEvent{Outcome: outcome{
				Title:   "Could not create problem",
				Message: err.Error(),
			}})
			return
		}

		details := []string{result.Path}
		if !result.Created {
			details = append(details, "existing solution file kept")
		}
		if result.ReadmeUpdated {
			details = append(details, "README.md updated")
		} else {
			details = append(details, "README.md already up to date")
		}
		if p.Difficulty != "" {
			details = append(details, p.Difficulty)
		}

		title := p.Title
		if p.ID != "" {
			title = p.ID + ". " + p.Title
		}

		sendEvent(ctx, events, doneEvent{Outcome: outcome{
			OK:      true,
			Title:   title,
			Message: "Problem ready.",
			Details: details,
		}})
	}()
}

func sendEvent(ctx context.Context, events chan<- any, event any) {
	select {
	case events <- event:
	case <-ctx.Done():
	}
}

func (a *app) handleEvent(event any) {
	switch event := event.(type) {
	case progressEvent:
		a.workingTitle = event.Title
	case doneEvent:
		a.cancelWork()
		a.outcome = event.Outcome
		a.screen = screenDone
	}
}

func (a *app) cancelWork() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.running = false
}

func isRune(key keyEvent, want rune) bool {
	return key.kind == keyRune && key.text == string(want)
}

func digitIndex(text string, count int) (int, bool) {
	if len(text) != 1 || text[0] < '1' || text[0] > '9' {
		return 0, false
	}
	index := int(text[0] - '1')
	if index >= count {
		return 0, false
	}
	return index, true
}
