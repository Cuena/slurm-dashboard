package main

import "github.com/charmbracelet/lipgloss"

func panelBorderShape() lipgloss.Border {
	if theme.Mode == ThemeLight {
		return lipgloss.ThickBorder()
	}
	return lipgloss.RoundedBorder()
}

var (
	subtle        = theme.TextMuted
	accentFill    = theme.Accent
	focusBorder   = theme.Focus
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
	helpKeyStyle  = lipgloss.NewStyle().
			Foreground(textStrong).
			Bold(true)
	helpDescStyle = lipgloss.NewStyle().
			Foreground(theme.TextDim)
	helpSepStyle = lipgloss.NewStyle().
			Foreground(panelBorder)
	helpEllipsisStyle = lipgloss.NewStyle().
				Foreground(subtle)

	// Top section styles
	metaPillStyle = lipgloss.NewStyle().
			Foreground(textStrong).
			Background(panelBgAccent).
			Padding(0, 1).
			Bold(true).
			Align(lipgloss.Center)

	metaMutedPillStyle = metaPillStyle.Copy().
				Foreground(subtle).
				Background(panelBg)

	metaAlertPillStyle = metaPillStyle.Copy().
				Background(danger).
				Foreground(textOnAccent).
				Bold(true)

	toolbarBrandStyle = metaPillStyle.Copy().
				Foreground(textOnAccent).
				Background(accentFill)

	toolbarActiveStyle = metaPillStyle.Copy().
				Foreground(textOnAccent).
				Background(accentFill)

	toolbarTabStyle = metaMutedPillStyle.Copy().
			Background(panelBg)

	toolbarMetaStyle = adaptiveToolbarMetaStyle()

	toolbarDividerStyle = toolbarMetaStyle.Copy()

	filterBoxStyle = lipgloss.NewStyle().
			Foreground(textStrong).
			Background(panelBgAccent).
			Padding(0, 1)

	filterHintStyle = lipgloss.NewStyle().
			Foreground(subtle)

	focusTagStyle = lipgloss.NewStyle().
			Foreground(textOnAccent).
			Background(accentFill).
			Padding(0, 1).
			Bold(true)

	// Main panels
	listStyle = lipgloss.NewStyle().
			Border(panelBorderShape()).
			BorderForeground(panelBorder).
			Background(panelBg).
			Padding(0, 1)

	detailsStyle = lipgloss.NewStyle().
			Border(panelBorderShape()).
			BorderForeground(panelBorder).
			Background(panelBg).
			Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(textStrong).
			Background(panelBg).
			Bold(true).
			MarginBottom(0)

	panelMetaStyle = lipgloss.NewStyle().
			Foreground(subtle).
			Background(panelBg)

	detailSummaryStyle = lipgloss.NewStyle().
				Background(panelBgAccent).
				Padding(0, 1).
				MarginBottom(1)

	detailSummaryLabelStyle = lipgloss.NewStyle().
				Foreground(subtle).
				Background(panelBgAccent)

	detailSummaryValueStyle = lipgloss.NewStyle().
				Foreground(textStrong).
				Background(panelBgAccent).
				Bold(true)

	detailInspectorStyle = lipgloss.NewStyle().
				PaddingTop(1)

	copyHintStyle = lipgloss.NewStyle().
			Foreground(subtle).
			Background(panelBgAccent)

	copyStatusStyle = lipgloss.NewStyle().
			Foreground(accentGreen).
			Bold(true)

	placeholderStyle = lipgloss.NewStyle().
				Foreground(subtle).
				Background(panelBg).
				Italic(true)

	dialogStyle = lipgloss.NewStyle().
			Border(panelBorderShape()).
			BorderForeground(accentPink).
			Background(panelBg).
			Padding(2, 4).
			Align(lipgloss.Center).
			Width(50)

	// Table Styles
	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(subtle).
				Background(panelBg).
				Bold(true).
				Align(lipgloss.Left).
				Padding(0, 1)

	tableCellStyle = lipgloss.NewStyle().
			Foreground(textStrong).
			Background(panelBg).
			Padding(0, 1)

	jobsTableSelectedCellStyle = tableCellStyle.Copy().
					Foreground(textStrong).
					Background(selectionBg)

	jobsTableSelectedMutedCellStyle = tableCellStyle.Copy().
					Foreground(textStrong).
					Background(panelBgAccent)

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

func adaptiveToolbarMetaStyle() lipgloss.Style {
	if theme.Mode == ThemeLight {
		return lipgloss.NewStyle().Foreground(theme.TextDim)
	}
	return lipgloss.NewStyle().Foreground(subtle)
}

func detailSelectedCellStyle(focused bool) lipgloss.Style {
	if focused {
		return tableCellStyle.Copy().
			Foreground(textOnAccent).
			Background(accentFill).
			Bold(true)
	}

	return tableCellStyle.Copy().
		Foreground(textStrong).
		Background(panelBgAccent)
}
