// Package busdays calcula prazos em dias úteis (segunda a sexta).
// Feriados não são considerados — ver requisito 6.4, decisão pendente.
package busdays

import "time"

// AddBusinessDays devolve a data n dias úteis após from (n > 0).
func AddBusinessDays(from time.Time, n int) time.Time {
	date := from
	for added := 0; added < n; {
		date = date.AddDate(0, 0, 1)
		if isBusinessDay(date) {
			added++
		}
	}
	return date
}

// SubBusinessDays devolve a data n dias úteis antes de from (n > 0).
func SubBusinessDays(from time.Time, n int) time.Time {
	date := from
	for subtracted := 0; subtracted < n; {
		date = date.AddDate(0, 0, -1)
		if isBusinessDay(date) {
			subtracted++
		}
	}
	return date
}

func isBusinessDay(date time.Time) bool {
	wd := date.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}
