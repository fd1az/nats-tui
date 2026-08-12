package preferences

import (
	"errors"
	"strings"
)

type Theme string

const (
	ThemeMidnight   Theme = "midnight"
	ThemeAccessible Theme = "accessible"
	ThemeMono       Theme = "mono"
)

type Bindings struct{ Quit, Help, Refresh, Commands, Bookmarks, Activity string }

func DefaultBindings() Bindings {
	return Bindings{Quit: "q", Help: "?", Refresh: "r", Commands: ":", Bookmarks: "b", Activity: "ctrl+g"}
}

type Preferences struct {
	Mouse bool
	Theme Theme
	Keys  Bindings
}

func New(mouse bool, theme string, keys Bindings) (Preferences, error) {
	parsed := Theme(strings.ToLower(strings.TrimSpace(theme)))
	if parsed == "" {
		parsed = ThemeMidnight
	}
	if parsed != ThemeMidnight && parsed != ThemeAccessible && parsed != ThemeMono {
		return Preferences{}, errors.New("theme must be midnight, accessible, or mono")
	}
	defaults := DefaultBindings()
	values := []*string{&keys.Quit, &keys.Help, &keys.Refresh, &keys.Commands, &keys.Bookmarks, &keys.Activity}
	fallbacks := []string{defaults.Quit, defaults.Help, defaults.Refresh, defaults.Commands, defaults.Bookmarks, defaults.Activity}
	seen := map[string]struct{}{}
	for i, value := range values {
		*value = strings.TrimSpace(*value)
		if *value == "" {
			*value = fallbacks[i]
		}
		if _, exists := seen[*value]; exists {
			return Preferences{}, errors.New("global key bindings must be unique")
		}
		seen[*value] = struct{}{}
	}
	return Preferences{Mouse: mouse, Theme: parsed, Keys: keys}, nil
}
