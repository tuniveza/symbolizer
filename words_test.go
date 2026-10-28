package main

import (
	"strings"
	"testing"
	"time"
)

func plainWords(ws []word) string {
	var out []string
	for _, w := range ws {
		out = append(out, w.text)
	}
	return strings.Join(out, " ")
}

func TestTimeAndDateWords(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 25, h, m, 0, 0, time.Local) }
	cases := map[time.Time]string{
		at(0, 54):  "six to one at night",
		at(0, 0):   "midnight",
		at(12, 0):  "noon",
		at(15, 0):  "three o'clock in the afternoon",
		at(15, 15): "quarter past three in the afternoon",
		at(15, 30): "half past three in the afternoon",
		at(15, 45): "quarter to four in the afternoon",
		at(9, 1):   "one past nine in the morning",
		at(9, 23):  "twenty-three past nine in the morning",
		at(18, 40): "twenty to seven in the evening",
		at(23, 50): "ten to midnight",
		at(11, 55): "five to noon",
		at(22, 5):  "five past ten at night",
	}
	for tm, want := range cases {
		if got := plainWords(timeWords(tm)); got != want {
			t.Errorf("%s: got %q, want %q", tm.Format("15:04"), got, want)
		}
	}
	dates := map[time.Time]string{
		time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local): "Sunday the twenty-fifth of October",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local):   "Thursday the first of January",
		time.Date(2026, 3, 22, 0, 0, 0, 0, time.Local):  "Sunday the twenty-second of March",
		time.Date(2026, 7, 30, 0, 0, 0, 0, time.Local):  "Thursday the thirtieth of July",
		time.Date(2026, 8, 31, 0, 0, 0, 0, time.Local):  "Monday the thirty-first of August",
		time.Date(2026, 2, 12, 0, 0, 0, 0, time.Local):  "Thursday the twelfth of February",
	}
	for d, want := range dates {
		if got := plainWords(dateWords(d)); got != want {
			t.Errorf("%s: got %q, want %q", d.Format("2006-01-02"), got, want)
		}
	}
}
