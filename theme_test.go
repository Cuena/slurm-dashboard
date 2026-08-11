package main

import "testing"

func TestResolveThemeMode(t *testing.T) {
	tests := []struct {
		name              string
		configured        ThemeMode
		hasDarkBackground bool
		want              ThemeMode
	}{
		{
			name:              "auto on light background",
			configured:        ThemeAuto,
			hasDarkBackground: false,
			want:              ThemeLight,
		},
		{
			name:              "auto on dark background",
			configured:        ThemeAuto,
			hasDarkBackground: true,
			want:              ThemeDark,
		},
		{
			name:              "explicit light overrides detection",
			configured:        ThemeLight,
			hasDarkBackground: true,
			want:              ThemeLight,
		},
		{
			name:              "explicit dark overrides detection",
			configured:        ThemeDark,
			hasDarkBackground: false,
			want:              ThemeDark,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveThemeMode(tt.configured, tt.hasDarkBackground); got != tt.want {
				t.Fatalf("resolveThemeMode(%q, %t) = %q, want %q", tt.configured, tt.hasDarkBackground, got, tt.want)
			}
		})
	}
}
