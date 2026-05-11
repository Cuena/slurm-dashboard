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
	Focus      lipgloss.TerminalColor
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
	surfacesRaw := os.Getenv(envSurfaces)
	surfaces := parseSurfaceMode(surfacesRaw)
	palette := parsePalette(os.Getenv(envPalette))

	// Transparent surfaces work well in dark terminals, but on a light terminal
	// they make the panels and chips feel washed out unless the user explicitly
	// opted into transparency.
	if mode == ThemeLight && strings.TrimSpace(surfacesRaw) == "" {
		surfaces = SurfaceSolid
	}

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
			TextMuted:    pickColor(mode, "#586069", "#9BA3BC"),
			TextStrong:   pickColor(mode, "#182033", "#F8FBFF"),
			TextOnAccent: lipgloss.Color("#F8FBFF"),
			TextDim:      pickColor(mode, "#666F79", "#7E869E"),
			Accent:       pickColor(mode, "#4F6475", "#2D4487"),
			Focus:        pickColor(mode, "#6B8194", "#779EF1"),
			Border:       pickColor(mode, "#7E8895", "#495A7D"),
			Surface:      pickSurface(mode, surfaces, "#F4F2EE", "#11121C"),
			SurfaceAlt:   pickSurface(mode, surfaces, "#FBFCFE", "#161A24"),
			AccentPink:   pickColor(mode, "#BA6B86", "#F06A9B"),
			AccentCyan:   pickColor(mode, "#3A8794", "#4DD0E1"),
			AccentOrange: pickColor(mode, "#B97B2E", "#FFB347"),
			AccentGreen:  pickColor(mode, "#2E7D63", "#2BD19F"),
			AccentBlue:   pickColor(mode, "#5B738C", "#5D9CFF"),
			Danger:       pickColor(mode, "#C94F5C", "#FF5F6D"),
			SelectionBg:  pickColor(mode, "#D9DFE5", "#314C73"),
			SelectionFg:  pickColor(mode, "#182033", "#F8FBFF"),
			SearchBg:     pickColor(mode, "#EBCB5A", "#FFD54F"),
			SearchFg:     pickColor(mode, "#182033", "#1A1A1A"),
		}
	default: // PaletteDraculaSoft
		// Dracula-inspired dark palette with a softer, more grounded light side so
		// light terminals do not look like an inverted dark theme.
		return Theme{
			Mode:         mode,
			Surfaces:     surfaces,
			Text:         lipgloss.NoColor{},
			TextMuted:    pickColor(mode, "#5E635F", "#B6B8C9"),
			TextStrong:   pickColor(mode, "#1A1B22", "#F8F8F2"),
			TextOnAccent: lipgloss.Color("#F8FBFF"),
			TextDim:      pickColor(mode, "#686D72", "#7D8297"),

			Accent: pickColor(mode, "#566775", "#2D4487"),
			Focus:  pickColor(mode, "#708493", "#779EF1"),
			Border: pickColor(mode, "#82888F", "#495A7D"),

			Surface:    pickSurface(mode, surfaces, "#F6F4EF", "#222633"),
			SurfaceAlt: pickSurface(mode, surfaces, "#FCFAF5", "#2B3140"),

			AccentPink:   pickColor(mode, "#B66C8D", "#FF79C6"),
			AccentCyan:   pickColor(mode, "#417F88", "#8BE9FD"),
			AccentOrange: pickColor(mode, "#B18042", "#FFB86C"),
			AccentGreen:  pickColor(mode, "#4B8562", "#50FA7B"),
			AccentBlue:   pickColor(mode, "#6C7E8F", "#6EA8FE"),
			Danger:       pickColor(mode, "#C9525B", "#FF5555"),

			SelectionBg: pickColor(mode, "#DBDED9", "#3A547A"),
			SelectionFg: pickColor(mode, "#1A1B22", "#F8F8F2"),

			SearchBg: pickColor(mode, "#E6E39A", "#F1FA8C"),
			SearchFg: pickColor(mode, "#282A36", "#282A36"),
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
