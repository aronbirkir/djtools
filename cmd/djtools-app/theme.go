package main

import (
	"image/color"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

type Palette struct {
	Bg       color.NRGBA
	Surface  color.NRGBA
	Surface2 color.NRGBA
	Border   color.NRGBA
	Fg       color.NRGBA
	Muted    color.NRGBA
	Accent   color.NRGBA
	Success  color.NRGBA
	Warn     color.NRGBA
	Error    color.NRGBA
}

var darkPalette = Palette{
	Bg:       rgb(0x121418),
	Surface:  rgb(0x1b1e25),
	Surface2: rgb(0x242832),
	Border:   rgb(0x2f3440),
	Fg:       rgb(0xe6e8eb),
	Muted:    rgb(0x8b93a1),
	Accent:   rgb(0x7c5cff),
	Success:  rgb(0x3ddc97),
	Warn:     rgb(0xf5b94a),
	Error:    rgb(0xff6b6b),
}

var lightPalette = Palette{
	Bg:       rgb(0xf4f5f8),
	Surface:  rgb(0xffffff),
	Surface2: rgb(0xf1f2f6),
	Border:   rgb(0xdfe2e8),
	Fg:       rgb(0x1a1d23),
	Muted:    rgb(0x6b7280),
	Accent:   rgb(0x6a4cf5),
	Success:  rgb(0x12a36b),
	Warn:     rgb(0xb7791f),
	Error:    rgb(0xd93b3b),
}

// pal is the palette used for the current frame.
var pal = &darkPalette

// ThemeMode is the user's theme preference, as stored in the config.
type ThemeMode string

const (
	ThemeSystem ThemeMode = "system"
	ThemeLight  ThemeMode = "light"
	ThemeDark   ThemeMode = "dark"
)

var themeModes = []ThemeMode{ThemeSystem, ThemeLight, ThemeDark}

func (m ThemeMode) Label() string {
	switch m {
	case ThemeLight:
		return "Light"
	case ThemeDark:
		return "Dark"
	default:
		return "System"
	}
}

func themeLabels() []string {
	out := make([]string, len(themeModes))
	for i, m := range themeModes {
		out[i] = m.Label()
	}
	return out
}

func themeIndex(m ThemeMode) int {
	for i, mode := range themeModes {
		if mode == m {
			return i
		}
	}
	return 0
}

func newTheme() *material.Theme {
	th := material.NewTheme()
	th.TextSize = unit.Sp(15)
	return th
}

// applyTheme picks the palette for this frame from the user's preference.
func applyTheme(th *material.Theme, mode ThemeMode, systemDark bool) {
	if mode == ThemeDark || (mode != ThemeLight && systemDark) {
		pal = &darkPalette
	} else {
		pal = &lightPalette
	}
	th.Palette = material.Palette{
		Bg:         pal.Bg,
		Fg:         pal.Fg,
		ContrastBg: pal.Accent,
		ContrastFg: rgb(0xffffff),
	}
}

// systemTheme tracks whether the OS is in dark mode.
type systemTheme struct {
	dark atomic.Bool
}

// watch polls the OS appearance and invalidates the window when it changes.
// Gio has no appearance-change event, so polling is the portable option.
func (s *systemTheme) watch(w *app.Window) {
	s.dark.Store(systemIsDark())
	go func() {
		for range time.Tick(3 * time.Second) {
			if d := systemIsDark(); d != s.dark.Load() {
				s.dark.Store(d)
				w.Invalidate()
			}
		}
	}()
}

func systemIsDark() bool {
	switch runtime.GOOS {
	case "darwin":
		// Prints "Dark" in dark mode; the key is absent in light mode.
		out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
		return err == nil && strings.TrimSpace(string(out)) == "Dark"
	case "windows":
		out, err := exec.Command("reg", "query",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
			"/v", "AppsUseLightTheme").Output()
		return err == nil && strings.Contains(string(out), "0x0")
	case "linux":
		out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
		return err == nil && strings.Contains(string(out), "dark")
	}
	return true
}
