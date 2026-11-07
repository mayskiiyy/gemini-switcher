package model

import "strings"

// ResolveLanguage preserves a manual selection and resolves automatic UI language.
func ResolveLanguage(preference string) string {
	if preference == "" || preference == "auto" {
		return systemLanguage()
	}
	return languageCode(preference)
}

func languageCode(tag string) string {
	code, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(tag, "_", "-")), "-")
	switch code {
	case "ru", "en", "de", "pl", "uk":
		return code
	}
	return "en"
}
