package render

import (
	"regexp"
	"strings"
)

// localeNames maps English month / weekday names (the words Go's
// time.Format emits) to their translation in the named locale.
// Adding a new locale = add one entry to this map; nothing else
// changes. Keys cover both long ("January", "Monday") and short
// ("Jan", "Mon") forms; "May" is intentionally one entry because the
// full and abbreviated English forms are identical.
//
// Locale codes are lowercase ISO 639-1; multi-region codes ("en-GB",
// "pt-BR") can be added when needed by registering the full code as
// its own key.
var localeNames = map[string]map[string]string{
	"nl": {
		"January": "januari", "February": "februari", "March": "maart",
		"April": "april", "May": "mei", "June": "juni",
		"July": "juli", "August": "augustus", "September": "september",
		"October": "oktober", "November": "november", "December": "december",
		"Jan": "jan", "Feb": "feb", "Mar": "mrt",
		"Apr": "apr", "Jun": "jun", "Jul": "jul",
		"Aug": "aug", "Sep": "sep", "Oct": "okt",
		"Nov": "nov", "Dec": "dec",
		"Monday": "maandag", "Tuesday": "dinsdag", "Wednesday": "woensdag",
		"Thursday": "donderdag", "Friday": "vrijdag", "Saturday": "zaterdag",
		"Sunday": "zondag",
		"Mon": "ma", "Tue": "di", "Wed": "wo",
		"Thu": "do", "Fri": "vr", "Sat": "za", "Sun": "zo",
	},
	"de": {
		"January": "Januar", "February": "Februar", "March": "März",
		"April": "April", "May": "Mai", "June": "Juni",
		"July": "Juli", "August": "August", "September": "September",
		"October": "Oktober", "November": "November", "December": "Dezember",
		"Jan": "Jan", "Feb": "Feb", "Mar": "Mär",
		"Apr": "Apr", "Jun": "Jun", "Jul": "Jul",
		"Aug": "Aug", "Sep": "Sep", "Oct": "Okt",
		"Nov": "Nov", "Dec": "Dez",
		"Monday": "Montag", "Tuesday": "Dienstag", "Wednesday": "Mittwoch",
		"Thursday": "Donnerstag", "Friday": "Freitag", "Saturday": "Samstag",
		"Sunday": "Sonntag",
		"Mon": "Mo", "Tue": "Di", "Wed": "Mi",
		"Thu": "Do", "Fri": "Fr", "Sat": "Sa", "Sun": "So",
	},
	"fr": {
		"January": "janvier", "February": "février", "March": "mars",
		"April": "avril", "May": "mai", "June": "juin",
		"July": "juillet", "August": "août", "September": "septembre",
		"October": "octobre", "November": "novembre", "December": "décembre",
		"Jan": "janv.", "Feb": "févr.", "Mar": "mars",
		"Apr": "avr.", "Jun": "juin", "Jul": "juil.",
		"Aug": "août", "Sep": "sept.", "Oct": "oct.",
		"Nov": "nov.", "Dec": "déc.",
		"Monday": "lundi", "Tuesday": "mardi", "Wednesday": "mercredi",
		"Thursday": "jeudi", "Friday": "vendredi", "Saturday": "samedi",
		"Sunday": "dimanche",
		"Mon": "lun.", "Tue": "mar.", "Wed": "mer.",
		"Thu": "jeu.", "Fri": "ven.", "Sat": "sam.", "Sun": "dim.",
	},
}

// localeRe matches any English month or weekday name (long forms
// listed first so the regex engine prefers `January` over `Jan`,
// `Monday` over `Mon`, etc.). `\b` ensures whole-word matches so
// "January" in input doesn't trigger a "Jan" substitution.
var localeRe = regexp.MustCompile(`\b(?:January|February|March|April|May|June|July|August|September|October|November|December|January|Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sep|Oct|Nov|Dec|Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday|Mon|Tue|Wed|Thu|Fri|Sat|Sun)\b`)

// translateDate runs a locale pass over a Go time.Format result.
// Empty / "en" / unknown locale → passthrough (no translation). The
// translation is a word-by-word regex replace so order in the original
// formatted string is preserved.
func translateDate(formatted, locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "" || locale == "en" {
		return formatted
	}
	table, ok := localeNames[locale]
	if !ok {
		return formatted
	}
	return localeRe.ReplaceAllStringFunc(formatted, func(match string) string {
		if t, ok := table[match]; ok {
			return t
		}
		return match
	})
}
