package doctor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorBorder  = lipgloss.Color("#313244")
	colorPrimary = lipgloss.Color("#89B4FA")
	colorAccent  = lipgloss.Color("#CBA6F7")
	colorSuccess = lipgloss.Color("#A6E3A1")
	colorWarning = lipgloss.Color("#F9E2AF")
	colorDanger  = lipgloss.Color("#F38BA8")
	colorMuted   = lipgloss.Color("#6C7086")
	colorText    = lipgloss.Color("#CDD6F4")

	badgeDoctor = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#11111B")).
			Background(colorPrimary).
			Padding(0, 1)

	badgePass = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#11111B")).
			Background(colorSuccess).
			Padding(0, 1)

	badgeFail = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#11111B")).
			Background(colorDanger).
			Padding(0, 1)

	cardBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2)
)

// RenderCard formats the tool diagnosis report into a terminal UI card.
func RenderCard(r *Report, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80
	}

	// 1. Header & Status Pill
	titlePill := badgeDoctor.Render(" DOCTOR ")
	subtext := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")).Render(" System & Tool Readiness Check")
	metaLeft := lipgloss.JoinHorizontal(lipgloss.Center, titlePill, subtext)

	var statusPill string
	if r.AllPassed {
		statusPill = badgePass.Render(fmt.Sprintf(" ● ALL %d READY ", len(r.Tools)))
	} else {
		missing := r.MissingTools()
		statusPill = badgeFail.Render(fmt.Sprintf(" ✖ %d MISSING ", len(missing)))
	}

	topSpacer := termWidth - lipgloss.Width(metaLeft) - lipgloss.Width(statusPill) - 6
	if topSpacer < 1 {
		topSpacer = 1
	}
	topBar := lipgloss.JoinHorizontal(lipgloss.Center, metaLeft, strings.Repeat(" ", topSpacer), statusPill)

	// 2. Metadata Lines
	var metaLines []string
	if r.ProjectName != "" {
		metaLines = append(metaLines, fmt.Sprintf("\033[2mProject:\033[0m     %s", r.ProjectName))
	}
	if r.ConfigPath != "" {
		metaLines = append(metaLines, fmt.Sprintf("\033[2mConfig File:\033[0m %s", r.ConfigPath))
	}

	// 3. Table of Tools
	colToolW := 14
	colStatusW := 12
	colVersionW := 16
	colPathW := termWidth - colToolW - colStatusW - colVersionW - 14
	if colPathW < 20 {
		colPathW = 20
	}

	tableHeader := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(
		fmt.Sprintf("  %-*s %-*s %-*s %s", colToolW, "TOOL", colStatusW, "STATUS", colVersionW, "VERSION", "PATH / SOURCE"),
	)
	var rows []string
	rows = append(rows, tableHeader)
	rows = append(rows, "  "+strings.Repeat("─", termWidth-10))

	for _, t := range r.Tools {
		var statBadge string
		if t.Found {
			if t.Source == "custom_path" {
				statBadge = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("● Custom")
			} else {
				statBadge = lipgloss.NewStyle().Bold(true).Foreground(colorSuccess).Render("● Ready")
			}
		} else {
			if t.Required {
				statBadge = lipgloss.NewStyle().Bold(true).Foreground(colorDanger).Render("✖ Missing")
			} else {
				statBadge = lipgloss.NewStyle().Foreground(colorWarning).Render("! Optional")
			}
		}

		ver := "-"
		if t.Version != "" {
			ver = t.Version
		}

		pathStr := t.Path
		if !t.Found {
			pathStr = lipgloss.NewStyle().Foreground(colorDanger).Render(t.Error)
		} else if t.Source == "custom_path" {
			pathStr = fmt.Sprintf("%s \033[2m(gryd.yaml)\033[0m", t.Path)
		}

		row := fmt.Sprintf("  %-*s %-*s %-*s %s",
			colToolW, t.Name,
			colStatusW, statBadge,
			colVersionW, ver,
			pathStr,
		)
		rows = append(rows, row)
	}

	// 4. Instructions if tools are missing
	var footerLines []string
	missing := r.MissingTools()
	if len(missing) > 0 {
		footerLines = append(footerLines, "")
		footerLines = append(footerLines, lipgloss.NewStyle().Bold(true).Foreground(colorWarning).Render("  How to fix missing tools:"))
		footerLines = append(footerLines, "  Install the tool on your system, or set its custom path in gryd.yaml:")
		footerLines = append(footerLines, "")
		snippet := GenerateConfigSnippet(r)
		for _, line := range strings.Split(snippet, "\n") {
			footerLines = append(footerLines, "    "+line)
		}
	} else {
		footerLines = append(footerLines, "")
		footerLines = append(footerLines, lipgloss.NewStyle().Foreground(colorSuccess).Render("  ✓ All required tools and runtime dependencies are verified!"))
	}

	// Combine all sections
	var body []string
	body = append(body, topBar, "")
	body = append(body, strings.Join(metaLines, "\n"), "")
	body = append(body, strings.Join(rows, "\n"))
	body = append(body, strings.Join(footerLines, "\n"))

	return cardBox.Width(termWidth - 2).Render(strings.Join(body, "\n"))
}

// GenerateConfigSnippet generates a ready-to-paste YAML snippet for missing tools.
func GenerateConfigSnippet(r *Report) string {
	missing := r.MissingTools()
	if len(missing) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "\033[1;36mtools:\033[0m")
	for _, m := range missing {
		samplePath := fmt.Sprintf("/path/to/%s", m.Name)
		lines = append(lines, fmt.Sprintf("  \033[36m%s:\033[0m %s", m.Name, samplePath))
	}
	return strings.Join(lines, "\n")
}

// RenderMissingToolsAlert renders a compact alert suitable for CLI pre-flight warnings.
func RenderMissingToolsAlert(missing []ToolStatus) string {
	var toolList []string
	for _, m := range missing {
		usages := ""
		if len(m.UsedBy) > 0 {
			usages = fmt.Sprintf(" (needed by %s)", strings.Join(m.UsedBy, ", "))
		}
		toolList = append(toolList, fmt.Sprintf("  • \033[1m%s\033[0m%s", m.Name, usages))
	}

	alertBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorDanger).
		Padding(1, 2)

	var snippetLines []string
	snippetLines = append(snippetLines, "\033[1;36mtools:\033[0m")
	for _, m := range missing {
		snippetLines = append(snippetLines, fmt.Sprintf("  \033[36m%s:\033[0m /path/to/%s", m.Name, m.Name))
	}

	content := fmt.Sprintf(`\033[1;31m✖ Missing Required Tool(s)\033[0m
Gryd could not detect the following executables required by your project:

%s

\033[2mTo resolve this, install the tool or set its absolute path in gryd.yaml:\033[0m

%s`, strings.Join(toolList, "\n"), strings.Join(snippetLines, "\n"))

	return alertBox.Render(content)
}
