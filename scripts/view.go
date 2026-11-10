package main

import "strings"

var spinners = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (a *app) render() error {
	width, height := a.term.size()
	lines := a.frame(width, height)

	var b strings.Builder
	b.Grow(width*height + 64)
	b.WriteString("\x1b[H")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(line)
	}
	b.WriteString("\x1b[0J")
	return a.term.write(b.String())
}

func (a *app) frame(width, height int) []string {
	if width < 32 || height < 10 {
		return a.smallFrame(width, height)
	}

	panel := a.panel(width, height)
	footer := truncateStyled(a.footer(), width)
	total := len(panel) + 2
	top := (height - total) / 2
	if top < 0 {
		top = 0
	}

	out := make([]string, 0, height)
	for i := 0; i < top && len(out) < height; i++ {
		out = append(out, "")
	}
	for _, line := range panel {
		if len(out) >= height {
			break
		}
		out = append(out, centerStyled(line, width))
	}
	if len(out) < height {
		out = append(out, "")
	}
	if len(out) < height {
		out = append(out, centerStyled(footer, width))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out[:height]
}

func (a *app) smallFrame(width, height int) []string {
	out := make([]string, height)
	message := "terminal too small"
	if width < len(message) {
		message = ""
	}
	for i := range out {
		line := ""
		if i == height/2 {
			line = centerStyled(a.theme.fg(colorFaint, message), width)
		}
		out[i] = padStyled(line, width)
	}
	return out
}

func (a *app) panel(width, height int) []string {
	pw := width - 6
	if pw > 64 {
		pw = 64
	}
	if pw < 28 {
		pw = width - 2
	}
	if pw < 20 {
		pw = width
	}

	switch a.screen {
	case screenInput:
		return a.inputPanel(pw, height)
	case screenLanguage:
		return a.languagePanel(pw, height)
	case screenWorking:
		return a.workingPanel(pw, height)
	case screenDone:
		return a.donePanel(pw, height)
	default:
		return nil
	}
}

func (a *app) inputPanel(pw, height int) []string {
	cw := pw - 4
	fieldWidth := cw - 2
	if fieldWidth < 1 {
		fieldWidth = 1
	}

	body := []string{
		"",
		a.theme.fg(colorDim, "Question link"),
		a.theme.fg(colorAccent, "❯ ") + a.field.view(fieldWidth, a.theme),
	}

	if a.inputErr != "" {
		body = append(body, "")
		for _, line := range wrapPlain(a.inputErr, cw) {
			body = append(body, a.theme.fg(colorRed, line))
		}
	} else {
		body = append(body, "", a.theme.fg(colorFaint, "Paste a LeetCode problem link."))
	}
	body = append(body, "")

	return a.renderPanel(pw, "New problem", body, height)
}

func (a *app) languagePanel(pw, height int) []string {
	cw := pw - 4
	body := []string{
		"",
		a.theme.fg(colorDim, "Choose a language"),
		"",
	}

	for i, lang := range languages {
		selected := i == a.lang
		marker := "  "
		name := a.theme.fg(colorText, lang.Name)
		if selected {
			marker = a.theme.fg(colorAccent, "▸ ")
			name = a.theme.bold(a.theme.fg(colorAccent, lang.Name))
		}

		line := marker + padStyled(name, 10) + "  " + a.theme.fg(colorFaint, "main."+lang.Ext)
		line = padStyled(line, cw)
		if selected {
			line = a.theme.bg(colorSelection, line)
		}
		body = append(body, line)
	}

	body = append(body, "")
	return a.renderPanel(pw, "Language", body, height)
}

func (a *app) workingPanel(pw, height int) []string {
	frame := spinners[a.spinner%len(spinners)]
	title := a.workingTitle
	if title == "" {
		title = "Working"
	}

	body := []string{
		"",
		a.theme.fg(colorAccent, frame) + " " + a.theme.bold(a.theme.fg(colorText, title)),
		"",
	}
	return a.renderPanel(pw, "Working", body, height)
}

func (a *app) donePanel(pw, height int) []string {
	color := colorGreen
	symbol := "✔"
	title := "Done"
	if !a.outcome.OK {
		color = colorRed
		symbol = "✖"
		title = "Error"
	}

	cw := pw - 4
	body := []string{
		"",
		a.theme.bold(a.theme.fg(color, symbol+"  "+a.outcome.Title)),
		"",
	}

	for _, line := range wrapPlain(a.outcome.Message, cw) {
		body = append(body, a.theme.fg(colorText, line))
	}
	if len(a.outcome.Details) > 0 {
		body = append(body, "")
		for _, detail := range a.outcome.Details {
			body = append(body, a.theme.fg(colorDim, truncatePlain(detail, cw)))
		}
	}
	body = append(body, "")

	return a.renderPanel(pw, title, body, height)
}

func (a *app) renderPanel(pw int, title string, body []string, height int) []string {
	inner := pw - 2
	cw := pw - 4
	if cw < 1 {
		cw = 1
	}

	maxBody := height - 5
	if maxBody < 1 {
		maxBody = 1
	}
	if len(body) > maxBody {
		body = body[:maxBody]
		if len(body) > 0 {
			body[len(body)-1] = a.theme.fg(colorFaint, "...")
		}
	}

	border := func(s string) string { return a.theme.fg(colorBorder, s) }
	line := func(content string) string {
		content = padStyled(content, cw)
		return border("│") + " " + content + " " + border("│")
	}

	out := make([]string, 0, len(body)+2)
	out = append(out, a.topBorder(pw, title))
	for _, text := range body {
		out = append(out, line(text))
	}
	out = append(out, border("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

func (a *app) topBorder(pw int, title string) string {
	inner := pw - 2
	border := func(s string) string { return a.theme.fg(colorBorder, s) }
	maxTitle := inner - 4
	if maxTitle < 1 {
		return border("╭" + strings.Repeat("─", inner) + "╮")
	}

	titleText := a.theme.bold(a.theme.fg(colorAccent, truncatePlain(title, maxTitle)))
	titleWidth := visibleWidth(titleText)
	space := inner - titleWidth - 2
	if space < 0 {
		space = 0
	}
	left := space / 2
	right := space - left

	return border("╭"+strings.Repeat("─", left)+" ") +
		titleText +
		border(" "+strings.Repeat("─", right)+"╮")
}

func (a *app) footer() string {
	switch a.screen {
	case screenInput:
		return a.hint("enter", "continue") + "   " + a.hint("esc", "quit")
	case screenLanguage:
		return a.hint("↑↓", "move") + "   " + a.hint("enter", "select") + "   " + a.hint("esc", "back")
	case screenWorking:
		return a.theme.fg(colorFaint, "working... ctrl+c to quit")
	case screenDone:
		if a.outcome.OK {
			return a.theme.fg(colorFaint, "press any key to exit")
		}
		return a.theme.fg(colorFaint, "press any key to go back")
	default:
		return ""
	}
}

func (a *app) hint(key, label string) string {
	return a.theme.fg(colorYellow, key) + " " + a.theme.fg(colorFaint, label)
}
