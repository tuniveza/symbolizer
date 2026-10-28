package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The info line above the status bar: the time and the date in words, and
// the weather.
//
// The weather comes from Open-Meteo (free, no key) through the same cache
// file the desktop bar's emberline-weather uses (~/.cache/emberline/weather.json),
// so both always agree and nothing is downloaded twice. If the file is
// missing or over ten minutes old, Symbolizer fetches it itself, the same way.

// ---- time and date in words ------------------------------------------------------

var smallNumbers = strings.Fields("zero one two three four five six seven eight nine ten eleven twelve " +
	"thirteen fourteen fifteen sixteen seventeen eighteen nineteen")
var tens = []string{"", "", "twenty", "thirty", "forty", "fifty"}

func numberWord(n int) string {
	if n < 20 {
		return smallNumbers[n]
	}
	if n%10 == 0 {
		return tens[n/10]
	}
	return tens[n/10] + "-" + smallNumbers[n%10]
}

var ordinalSmall = strings.Fields("zeroth first second third fourth fifth sixth seventh eighth ninth tenth " +
	"eleventh twelfth thirteenth fourteenth fifteenth sixteenth seventeenth eighteenth nineteenth")

func ordinalWord(n int) string {
	switch {
	case n < 20:
		return ordinalSmall[n]
	case n%10 == 0:
		return strings.TrimSuffix(tens[n/10], "y") + "ieth"
	}
	return tens[n/10] + "-" + ordinalSmall[n%10]
}

// word is a word of the info line; strong words are drawn bright and in
// capitals, like the desktop bar's clock ("SIX to ONE").
type word struct {
	text   string
	strong bool
}

// timeWords: 00:54 -> six to one at night; 15:30 -> half past three in the
// afternoon; 12:00 -> noon.
func timeWords(t time.Time) []word {
	h, m := t.Hour(), t.Minute()
	period := func(h int) word {
		switch {
		case h < 5:
			return word{"at night", false}
		case h < 12:
			return word{"in the morning", false}
		case h < 17:
			return word{"in the afternoon", false}
		case h < 21:
			return word{"in the evening", false}
		}
		return word{"at night", false}
	}
	hourWord := func(h int) string {
		h %= 12
		if h == 0 {
			h = 12
		}
		return numberWord(h)
	}
	switch {
	case m == 0 && h == 0:
		return []word{{"midnight", true}}
	case m == 0 && h == 12:
		return []word{{"noon", true}}
	case m == 0:
		return []word{{hourWord(h), true}, {"o'clock", false}, period(h)}
	}
	var out []word
	next := h
	switch {
	case m == 15:
		out = []word{{"quarter", true}, {"past", false}}
	case m == 30:
		out = []word{{"half", true}, {"past", false}}
	case m == 45:
		out = []word{{"quarter", true}, {"to", false}}
		next = h + 1
	case m < 30:
		out = []word{{numberWord(m), true}, {"past", false}}
	default:
		out = []word{{numberWord(60 - m), true}, {"to", false}}
		next = h + 1
	}
	if next%24 == 0 && next != h {
		return append(out, word{"midnight", true})
	}
	if next == 12 && next != h {
		return append(out, word{"noon", true})
	}
	return append(out, word{hourWord(next), true}, period(h))
}

// dateWords: Sunday the twenty-fifth of October.
func dateWords(t time.Time) []word {
	return []word{{t.Weekday().String(), true}, {"the", false}, {ordinalWord(t.Day()), true},
		{"of", false}, {t.Month().String(), true}}
}

// ---- weather ---------------------------------------------------------------------------

type weatherData struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Fetched   float64 `json:"_fetched"`
	Current   struct {
		Temp     float64 `json:"temperature_2m"`
		Feels    float64 `json:"apparent_temperature"`
		Humidity float64 `json:"relative_humidity_2m"`
		IsDay    int     `json:"is_day"`
		Code     int     `json:"weather_code"`
		Wind     float64 `json:"wind_speed_10m"`
		WindDir  float64 `json:"wind_direction_10m"`
		Gusts    float64 `json:"wind_gusts_10m"`
	} `json:"current"`
	Daily struct {
		Max  []float64 `json:"temperature_2m_max"`
		Min  []float64 `json:"temperature_2m_min"`
		Rain []float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
	Hourly struct {
		Time []string  `json:"time"`
		Rain []float64 `json:"precipitation_probability"`
	} `json:"hourly"`
	Place string `json:"-"`
}

type weatherMsg struct{ w *weatherData }
type minuteTick struct{}
type weatherTick struct{}

// WMO weather codes: words, day icon, night icon (Nerd Font, as the bar uses),
// and a plain symbol for other terminals.
var wmo = map[int]struct{ words, day, night, plain string }{
	0: {"Clear", "\U000f0599", "\U000f0594", "☀"}, 1: {"Mainly clear", "\U000f0599", "\U000f0594", "☀"},
	2: {"Partly cloudy", "\U000f0595", "\U000f0f31", "☁"}, 3: {"Overcast", "\U000f0590", "\U000f0590", "☁"},
	45: {"Fog", "\U000f0591", "\U000f0591", "≋"}, 48: {"Freezing fog", "\U000f0591", "\U000f0591", "≋"},
	51: {"Light drizzle", "\U000f0f33", "\U000f0597", "☂"}, 53: {"Drizzle", "\U000f0f33", "\U000f0597", "☂"},
	55: {"Heavy drizzle", "\U000f0597", "\U000f0597", "☂"}, 56: {"Freezing drizzle", "\U000f067f", "\U000f067f", "☂"},
	57: {"Freezing drizzle", "\U000f067f", "\U000f067f", "☂"}, 61: {"Light rain", "\U000f0597", "\U000f0597", "☂"},
	63: {"Rain", "\U000f0597", "\U000f0597", "☂"}, 65: {"Heavy rain", "\U000f0596", "\U000f0596", "☂"},
	66: {"Freezing rain", "\U000f067f", "\U000f067f", "☂"}, 67: {"Freezing rain", "\U000f067f", "\U000f067f", "☂"},
	71: {"Light snow", "\U000f0598", "\U000f0598", "❄"}, 73: {"Snow", "\U000f0598", "\U000f0598", "❄"},
	75: {"Heavy snow", "\U000f0f36", "\U000f0f36", "❄"}, 77: {"Snow grains", "\U000f0598", "\U000f0598", "❄"},
	80: {"Light showers", "\U000f0f33", "\U000f0597", "☂"}, 81: {"Showers", "\U000f0597", "\U000f0597", "☂"},
	82: {"Violent showers", "\U000f0596", "\U000f0596", "☂"}, 85: {"Snow showers", "\U000f0598", "\U000f0598", "❄"},
	86: {"Heavy snow showers", "\U000f0f36", "\U000f0f36", "❄"}, 95: {"Thunderstorm", "\U000f067e", "\U000f067e", "⚡"},
	96: {"Thunderstorm, hail", "\U000f0592", "\U000f0592", "⚡"}, 99: {"Thunderstorm, heavy hail", "\U000f0592", "\U000f0592", "⚡"},
}

var compass = strings.Fields("N NNE NE ENE E ESE SE SSE S SSW SW WSW W WNW NW NNW")

func weatherCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "emberline", "weather.json")
}

// place is where the weather is for: ~/.config/emberline/location
// ("LAT LON name"), else the timezone's city, else London — as the bar does.
func place() (lat, lon float64, name string) {
	home, _ := os.UserHomeDir()
	if data, err := os.ReadFile(filepath.Join(home, ".config", "emberline", "location")); err == nil {
		f := strings.Fields(string(data))
		if len(f) >= 2 {
			a, e1 := strconv.ParseFloat(f[0], 64)
			b, e2 := strconv.ParseFloat(f[1], 64)
			if e1 == nil && e2 == nil {
				name = strings.Join(f[2:], " ")
				if name == "" {
					name = "Home"
				}
				return a, b, name
			}
		}
	}
	tz := os.Getenv("TZ")
	if tz == "" {
		if real, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
			if i := strings.Index(real, "zoneinfo/"); i >= 0 {
				tz = real[i+9:]
			}
		}
	}
	if data, err := os.ReadFile("/usr/share/zoneinfo/zone1970.tab"); err == nil && tz != "" {
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Split(line, "\t")
			if len(f) >= 3 && f[2] == tz {
				c := f[1]
				i := max(strings.LastIndex(c, "+"), strings.LastIndex(c, "-"))
				if i > 0 {
					city := tz[strings.LastIndex(tz, "/")+1:]
					return dms(c[:i]), dms(c[i:]), strings.ReplaceAll(city, "_", " ")
				}
			}
		}
	}
	return 51.5, -0.13, "London"
}

// dms turns zone1970.tab's +DDMM[SS] / +DDDMM[SS] into degrees.
func dms(s string) float64 {
	sign := 1.0
	if s[0] == '-' {
		sign = -1
	}
	d := s[1:]
	degLen := 3
	if len(d) == 4 || len(d) == 6 {
		degLen = 2
	}
	deg, _ := strconv.Atoi(d[:degLen])
	mins, _ := strconv.Atoi(d[degLen : degLen+2])
	secs := 0
	if len(d) > degLen+2 {
		secs, _ = strconv.Atoi(d[degLen+2:])
	}
	return sign * (float64(deg) + float64(mins)/60 + float64(secs)/3600)
}

// loadWeather reads the shared cache, fetching first if it is missing,
// over ten minutes old, or for another place. Runs in the background.
func loadWeather() tea.Cmd {
	return func() tea.Msg {
		lat, lon, name := place()
		w := readWeatherCache()
		fresh := w != nil && time.Since(time.Unix(int64(w.Fetched), 0)) < 10*time.Minute &&
			math.Abs(w.Latitude-lat) < 0.1 && math.Abs(w.Longitude-lon) < 0.1
		if !fresh && os.Getenv("SY_NO_NETWORK") == "" {
			if f := fetchWeather(lat, lon); f != nil {
				w = f
			}
		}
		if w == nil {
			return weatherMsg{nil}
		}
		w.Place = name
		return weatherMsg{w}
	}
}

func readWeatherCache() *weatherData {
	data, err := os.ReadFile(weatherCachePath())
	if err != nil {
		return nil
	}
	var w weatherData
	if json.Unmarshal(data, &w) != nil {
		return nil
	}
	return &w
}

// fetchWeather asks Open-Meteo, with the same questions as the desktop bar,
// and writes the shared cache.
func fetchWeather(lat, lon float64) *weatherData {
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(math.Round(lat*1000)/1000, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(math.Round(lon*1000)/1000, 'f', -1, 64))
	q.Set("timezone", "auto")
	q.Set("wind_speed_unit", "mph")
	q.Set("forecast_days", "5")
	q.Set("current", "temperature_2m,apparent_temperature,relative_humidity_2m,is_day,weather_code,"+
		"pressure_msl,wind_speed_10m,wind_direction_10m,wind_gusts_10m,precipitation")
	q.Set("hourly", "temperature_2m,precipitation_probability,weather_code,is_day,uv_index")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,"+
		"precipitation_sum,sunrise,sunset,uv_index_max")
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://api.open-meteo.com/v1/forecast?" + q.Encode())
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var raw map[string]any
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&raw) != nil {
		return nil
	}
	raw["_fetched"] = float64(time.Now().Unix())
	data, _ := json.Marshal(raw)
	path := weatherCachePath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".sy-tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, path)
	}
	var w weatherData
	if json.Unmarshal(data, &w) != nil {
		return nil
	}
	return &w
}

// rainSoon is the highest chance of rain in the next six hours.
func (w *weatherData) rainSoon(now time.Time) (float64, bool) {
	best, found := 0.0, false
	for i, ts := range w.Hourly.Time {
		t, err := time.ParseInLocation("2006-01-02T15:04", ts, time.Local)
		if err != nil || i >= len(w.Hourly.Rain) {
			continue
		}
		if !t.Before(now.Truncate(time.Hour)) && t.Before(now.Add(6*time.Hour)) {
			best, found = math.Max(best, w.Hourly.Rain[i]), true
		}
	}
	return best, found
}

// weatherParts is the weather as pieces, most important first, so a narrow
// window can drop the end ones.
func (w *weatherData) parts(now time.Time) []string {
	c := w.Current
	info, ok := wmo[c.Code]
	if !ok {
		info = wmo[3]
		info.words = "Weather code " + strconv.Itoa(c.Code)
	}
	glyph := info.plain
	if nerdIcons {
		glyph = info.day
		if c.IsDay == 0 {
			glyph = info.night
		}
	}
	deg := func(v float64) string { return strconv.Itoa(int(math.Round(v))) + "°" }
	out := []string{glyph + " " + info.words + " " + deg(c.Temp), "feels " + deg(c.Feels)}
	if len(w.Daily.Max) > 0 && len(w.Daily.Min) > 0 {
		out = append(out, "high "+deg(w.Daily.Max[0])+" low "+deg(w.Daily.Min[0]))
	}
	wind := fmt.Sprintf("wind %d mph %s", int(math.Round(c.Wind)), compass[int(math.Round(c.WindDir/22.5))%16])
	if c.Gusts > c.Wind+5 {
		wind += fmt.Sprintf(", gusts %d", int(math.Round(c.Gusts)))
	}
	out = append(out, wind, fmt.Sprintf("humidity %d%%", int(math.Round(c.Humidity))))
	if p, ok := w.rainSoon(now); ok {
		out = append(out, fmt.Sprintf("rain %d%% next 6 h", int(math.Round(p))))
	}
	if w.Place != "" {
		out = append(out, w.Place)
	}
	if age := time.Since(time.Unix(int64(w.Fetched), 0)); w.Fetched > 0 && age > 3*time.Hour {
		out = append(out, "("+strconv.Itoa(int(age.Hours()))+" h old)")
	}
	return out
}

// ---- ticking -------------------------------------------------------------------------------

// nextMinute wakes the screen at the start of the next minute, so the time
// in words changes exactly when the clock does.
func nextMinute() tea.Cmd {
	now := time.Now()
	wait := now.Truncate(time.Minute).Add(time.Minute).Sub(now) + 20*time.Millisecond
	return tea.Tick(wait, func(time.Time) tea.Msg { return minuteTick{} })
}

func weatherEvery() tea.Cmd {
	return tea.Tick(5*time.Minute, func(time.Time) tea.Msg { return weatherTick{} })
}

// ---- drawing -------------------------------------------------------------------------------

func (m *Model) styledWords(ws []word, bg string) string {
	var parts []string
	for _, w := range ws {
		if w.strong {
			parts = append(parts, lipgloss.NewStyle().Background(hex(bg)).Foreground(hex(cEmber)).Bold(true).Render(strings.ToUpper(w.text)))
		} else {
			parts = append(parts, lipgloss.NewStyle().Background(hex(bg)).Foreground(hex(cDim)).Render(w.text))
		}
	}
	return strings.Join(parts, lipgloss.NewStyle().Background(hex(bg)).Render(" "))
}

// infoLine is drawn just above the status bar.
func (m *Model) infoLine() string {
	bg := cBarBG
	sp := func(s string) string { return lipgloss.NewStyle().Background(hex(bg)).Render(s) }
	dot := lipgloss.NewStyle().Background(hex(bg)).Foreground(hex(cGutterLine)).Render("  ·  ")
	now := time.Now()
	clock := m.styledWords(timeWords(now), bg)
	date := m.styledWords(dateWords(now), bg)
	var weather []string
	if m.weather != nil {
		weather = m.weather.parts(now)
	}
	wstyle := lipgloss.NewStyle().Background(hex(bg)).Foreground(hex(cStatusText))
	render := func(withDate bool, nWeather int) string {
		left := sp(" ") + clock
		if withDate {
			left += dot + date
		}
		right := ""
		if nWeather > 0 {
			right = wstyle.Render(strings.Join(weather[:nWeather], " · ")) + sp(" ")
		}
		gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			return ""
		}
		return left + sp(strings.Repeat(" ", gap)) + right
	}
	// Shrink on narrow windows: the extra weather details first, then the
	// date, then the rest of the weather.
	for n := len(weather); n >= min(2, len(weather)); n-- {
		if s := render(true, n); s != "" {
			return s
		}
	}
	for n := len(weather); n >= 0; n-- {
		if s := render(false, n); s != "" {
			return s
		}
	}
	return fit(sp(" ")+clock, m.width, lipgloss.NewStyle().Background(hex(bg)))
}
