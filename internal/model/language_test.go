package model

import "testing"

func TestLanguageSelection(t *testing.T) {
	for _, tc := range []struct{ preference, want string }{
		{"ru-RU", "ru"}, {"EN_us", "en"}, {"de-AT", "de"},
		{"pl", "pl"}, {"uk-UA", "uk"}, {"fr-FR", "en"}, {"invalid", "en"},
	} {
		if got := ResolveLanguage(tc.preference); got != tc.want {
			t.Errorf("ResolveLanguage(%q) = %q, want %q", tc.preference, got, tc.want)
		}
	}
	if DefaultConfig().Language != "auto" {
		t.Fatal("new installations must follow the system UI language")
	}
	if ResolveLanguage("") != ResolveLanguage("auto") {
		t.Fatal("missing language in older configs must use automatic selection")
	}
	switch got := ResolveLanguage("auto"); got {
	case "ru", "en", "de", "pl", "uk":
	default:
		t.Fatalf("unsupported system language was not resolved to English: %q", got)
	}
}
