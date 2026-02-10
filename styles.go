package main

import "github.com/charmbracelet/lipgloss"

var (
	subtle        = theme.TextMuted
	highlight     = theme.Accent
	panelBorder   = theme.Border
	panelBg       = theme.Surface
	panelBgAccent = theme.SurfaceAlt
	accentPink    = theme.AccentPink
	accentCyan    = theme.AccentCyan
	accentOrange  = theme.AccentOrange
	accentGreen   = theme.AccentGreen
	accentBlue    = theme.AccentBlue
	danger        = theme.Danger
	textStrong    = theme.TextStrong
	textOnAccent  = theme.TextOnAccent
	selectionBg   = theme.SelectionBg
	selectionFg   = theme.SelectionFg

	// Top section styles
	metaPillStyle = lipgloss.NewStyle().
			Foreground(highlight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(panelBorder).
			Padding(0, 1).
			Bold(true).
			Align(lipgloss.Center)

	metaMutedPillStyle = metaPillStyle.Copy().
				Foreground(subtle).
				BorderForeground(panelBorder)

	metaAlertPillStyle = metaPillStyle.Copy().
				Background(accentPink).
				Foreground(textOnAccent).
				BorderForeground(accentPink)

	filterBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(panelBorder).
			Background(panelBg).
			Padding(0, 1)

	filterHintStyle = lipgloss.NewStyle().
			Foreground(subtle)

	focusTagStyle = lipgloss.NewStyle().
			Foreground(textOnAccent).
			Background(highlight).
			Padding(0, 1).
			Bold(true).
			MarginLeft(1)

	summaryChipStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(panelBorder).
				Padding(0, 1).
				Align(lipgloss.Left).
				MarginRight(1)

	summaryLabelStyle = lipgloss.NewStyle().
				Foreground(subtle).
				Bold(true)

	summaryValueStyle = lipgloss.NewStyle().
				Foreground(textStrong).
				Bold(true)

	// Main panels
	listStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(panelBorder).
			Background(panelBgAccent).
			Padding(1, 2)

	detailsStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(panelBorder).
			Background(panelBgAccent).
			Padding(1, 2)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(subtle).
			Bold(true).
			MarginBottom(1)

	detailInspectorStyle = lipgloss.NewStyle().
				PaddingTop(1).
				Background(panelBgAccent)

	copyHintStyle = lipgloss.NewStyle().
			Foreground(subtle)

	copyStatusStyle = lipgloss.NewStyle().
			Foreground(accentGreen).
			Bold(true)

	placeholderStyle = lipgloss.NewStyle().
				Foreground(subtle).
				Italic(true)

	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentPink).
			Background(panelBg).
			Padding(2, 4).
			Align(lipgloss.Center).
			Width(50)

	// Table Styles
	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(subtle).
				Bold(true).
				Align(lipgloss.Left).
				Padding(0, 1)

	tableSelectedStyle = lipgloss.NewStyle().
				Foreground(selectionFg).
				Background(selectionBg).
				Padding(0, 1)

	statusBadgeStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Bold(true).
				Foreground(textOnAccent)
)

var statusColorMap = map[string]lipgloss.TerminalColor{
	"R":   accentGreen,
	"CG":  accentGreen,
	"PD":  accentOrange,
	"CF":  accentOrange,
	"PR":  accentOrange,
	"RQ":  accentOrange,
	"RS":  accentOrange,
	"S":   accentOrange,
	"ST":  accentOrange,
	"RH":  accentOrange,
	"RF":  accentOrange,
	"CD":  accentBlue,
	"CA":  accentPink,
	"F":   danger,
	"TO":  danger,
	"NF":  danger,
	"OOM": danger,
}

func statusColor(state string) lipgloss.TerminalColor {
	if c, ok := statusColorMap[state]; ok {
		return c
	}
	return theme.TextDim
}
