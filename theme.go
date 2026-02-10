package main

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	envTheme    = "SLURM_DASHBOARD_THEME"
	envSurfaces = "SLURM_DASHBOARD_SURFACES"
	envPalette  = "SLURM_DASHBOARD_PALETTE"
)

type ThemeMode string

const (
	ThemeAuto  ThemeMode = "auto"
	ThemeDark  ThemeMode = "dark"
	ThemeLight ThemeMode = "light"
)

type SurfaceMode string

const (
	SurfaceSolid       SurfaceMode = "solid"
	SurfaceTransparent SurfaceMode = "transparent"
)

type Palette string

const (
	PaletteDraculaSoft Palette = "dracula-soft"
	PaletteClassic     Palette = "classic"
)

type Theme struct {
	Mode     ThemeMode
	Surfaces SurfaceMode

	Text         lipgloss.TerminalColor
	TextMuted    lipgloss.TerminalColor
	TextStrong   lipgloss.TerminalColor
	TextOnAccent lipgloss.TerminalColor
	TextDim      lipgloss.TerminalColor

	Accent     lipgloss.TerminalColor
	Border     lipgloss.TerminalColor
	Surface    lipgloss.TerminalColor
	SurfaceAlt lipgloss.TerminalColor

	AccentPink   lipgloss.TerminalColor
	AccentCyan   lipgloss.TerminalColor
	AccentOrange lipgloss.TerminalColor
	AccentGreen  lipgloss.TerminalColor
	AccentBlue   lipgloss.TerminalColor
	Danger       lipgloss.TerminalColor

	SelectionBg lipgloss.TerminalColor
	SelectionFg lipgloss.TerminalColor

	SearchBg lipgloss.TerminalColor
	SearchFg lipgloss.TerminalColor
}

var theme = loadTheme()

func loadTheme() Theme {
	mode := parseThemeMode(os.Getenv(envTheme))
	surfaces := parseSurfaceMode(os.Getenv(envSurfaces))
	palette := parsePalette(os.Getenv(envPalette))

	if mode == ThemeDark {
		lipgloss.SetHasDarkBackground(true)
	} else if mode == ThemeLight {
		lipgloss.SetHasDarkBackground(false)
	}

	return newTheme(mode, surfaces, palette)
}

func parseThemeMode(value string) ThemeMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "dark":
		return ThemeDark
	case "light":
		return ThemeLight
	case "auto", "":
		return ThemeAuto
	default:
		return ThemeAuto
	}
}

func parseSurfaceMode(value string) SurfaceMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "solid":
		return SurfaceSolid
	case "transparent", "":
		return SurfaceTransparent
	default:
		return SurfaceTransparent
	}
}

func parsePalette(value string) Palette {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "classic":
		return PaletteClassic
	case "dracula-soft", "":
		return PaletteDraculaSoft
	default:
		return PaletteDraculaSoft
	}
}

func newTheme(mode ThemeMode, surfaces SurfaceMode, palette Palette) Theme {
	switch palette {
	case PaletteClassic:
		return Theme{
			Mode:         mode,
			Surfaces:     surfaces,
			Text:         lipgloss.NoColor{},
			TextMuted:    pickColor(mode, "#6B7394", "#9BA3BC"),
			TextStrong:   pickColor(mode, "#0B0D19", "#F8FBFF"),
			TextOnAccent: pickColor(mode, "#F8FBFF", "#0B0D19"),
			TextDim:      pickColor(mode, "#8890A8", "#7E869E"),
			Accent:       pickColor(mode, "#6C63FF", "#A8A0FF"),
			Border:       pickColor(mode, "#D7DBF5", "#454B66"),
			Surface:      pickSurface(mode, surfaces, "#F7F8FE", "#11121C"),
			SurfaceAlt:   pickSurface(mode, surfaces, "#FFFFFF", "#1A1C28"),
			AccentPink:   lipgloss.Color("#F06A9B"),
			AccentCyan:   lipgloss.Color("#4DD0E1"),
			AccentOrange: lipgloss.Color("#FFB347"),
			AccentGreen:  lipgloss.Color("#2BD19F"),
			AccentBlue:   lipgloss.Color("#5D9CFF"),
			Danger:       lipgloss.Color("#FF5F6D"),
			SelectionBg:  pickColor(mode, "#E6E9F6", "#3B3F5C"),
			SelectionFg:  pickColor(mode, "#0B0D19", "#F5F7FF"),
			SearchBg:     lipgloss.Color("#FFD54F"),
			SearchFg:     lipgloss.Color("#1A1A1A"),
		}
	default: // PaletteDraculaSoft
		// Dracula-inspired dark palette with slightly muted accent usage.
		// Light side stays close to the classic palette so auto-mode remains usable.
		return Theme{
			Mode:         mode,
			Surfaces:     surfaces,
			Text:         lipgloss.NoColor{},
			TextMuted:    pickColor(mode, "#6B7394", "#B6B8C9"),
			TextStrong:   pickColor(mode, "#0B0D19", "#F8F8F2"),
			TextOnAccent: pickColor(mode, "#F8FBFF", "#282A36"),
			TextDim:      pickColor(mode, "#8890A8", "#7D8297"),

			// Keep accent softer and reserve it for focus/selection.
			Accent: pickColor(mode, "#6C63FF", "#A78BFA"),

			// Use a neutral border (Dracula selection-ish), not the accent.
			Border: pickColor(mode, "#D7DBF5", "#44475A"),

			Surface:    pickSurface(mode, surfaces, "#F7F8FE", "#282A36"),
			SurfaceAlt: pickSurface(mode, surfaces, "#FFFFFF", "#2F3344"),

			AccentPink:   lipgloss.Color("#FF79C6"),
			AccentCyan:   lipgloss.Color("#8BE9FD"),
			AccentOrange: lipgloss.Color("#FFB86C"),
			AccentGreen:  lipgloss.Color("#50FA7B"),
			AccentBlue:   lipgloss.Color("#6EA8FE"),
			Danger:       lipgloss.Color("#FF5555"),

			SelectionBg: pickColor(mode, "#E6E9F6", "#44475A"),
			SelectionFg: pickColor(mode, "#0B0D19", "#F8F8F2"),

			SearchBg: lipgloss.Color("#F1FA8C"),
			SearchFg: lipgloss.Color("#282A36"),
		}
	}
}

func pickColor(mode ThemeMode, light, dark string) lipgloss.TerminalColor {
	switch mode {
	case ThemeDark:
		return lipgloss.Color(dark)
	case ThemeLight:
		return lipgloss.Color(light)
	default:
		return lipgloss.AdaptiveColor{Light: light, Dark: dark}
	}
}

func pickSurface(mode ThemeMode, surfaces SurfaceMode, light, dark string) lipgloss.TerminalColor {
	if surfaces == SurfaceTransparent {
		return lipgloss.NoColor{}
	}
	return pickColor(mode, light, dark)
}
