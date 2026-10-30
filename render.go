package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// ---------- 色付け ----------
//
// 桁揃えは「色を付ける前の素の文字列」で計算する。
// 先に pad してから wrap するのが約束事（ANSI コードは表示幅 0 なので）。

type styler struct {
	on   bool
	unit string // "°C" or "°F"
}

func (s styler) wrap(code, text string) string {
	if !s.on || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s styler) dim(t string) string  { return s.wrap("2", t) }
func (s styler) bold(t string) string { return s.wrap("1", t) }
func (s styler) cyan(t string) string { return s.wrap("36", t) }

func tempColorCode(t float64) string {
	switch {
	case t < 0:
		return "38;5;33"
	case t < 5:
		return "38;5;45"
	case t < 10:
		return "38;5;51"
	case t < 15:
		return "38;5;48"
	case t < 20:
		return "38;5;84"
	case t < 25:
		return "38;5;226"
	case t < 30:
		return "38;5;214"
	case t < 35:
		return "38;5;208"
	}
	return "38;5;196"
}

// 降水系は水色〜青でまとめる（確率が高いほど濃い青）。
func probColorCode(p float64) string {
	switch {
	case p >= 80:
		return "38;5;27"
	case p >= 50:
		return "38;5;33"
	case p >= 20:
		return "38;5;39"
	}
	return "2"
}

// precipColorCode は降水量の色。降っていなければ dim。
func precipColorCode(mm float64) string {
	if mm <= 0 {
		return "2"
	}
	return "38;5;39"
}

// tempCell は気温を色付き・右詰めで返す。
func (s styler) tempCell(t float64, width int) string {
	return s.wrap(tempColorCode(t), padLeft(s.tempStr(t), width))
}

func (s styler) tempStr(t float64) string { return fmt.Sprintf("%.1f%s", t, s.unit) }

// probCell は降水確率＋バーを色付き・左詰めで返す。
func (s styler) probCell(p float64, barW, width int) string {
	plain := padRight(padLeft(formatProb(p), 4)+" "+precipBar(p, barW), width)
	return s.wrap(probColorCode(p), plain)
}

// precipCell は降水量を青色・右詰めで返す（0mm は dim）。
func (s styler) precipCell(mm float64, width int) string {
	return s.wrap(precipColorCode(mm), padLeft(formatPrecip(mm), width))
}

// ---------- 表示幅（East Asian Width） ----------

func dispWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

func runeWidth(r rune) int {
	switch {
	case r == 0x200B || r == 0xFE0F:
		return 0
	case r < 0x20:
		return 0
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0x303E,
		r >= 0x3041 && r <= 0x33FF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x4E00 && r <= 0x9FFF,
		r >= 0xA000 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}

func padRight(s string, width int) string {
	if w := dispWidth(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

func padLeft(s string, width int) string {
	if w := dispWidth(s); w < width {
		return strings.Repeat(" ", width-w) + s
	}
	return s
}

// ---------- 小物 ----------

func formatProb(p float64) string { return fmt.Sprintf("%d%%", int(math.Round(p))) }

func precipBar(p float64, width int) string {
	filled := int(math.Round(p / 100 * float64(width)))
	filled = min(max(filled, 0), width)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func formatPrecip(mm float64) string {
	if mm <= 0 {
		return "0.0mm"
	}
	if mm < 10 {
		return fmt.Sprintf("%.1fmm", mm)
	}
	return fmt.Sprintf("%.0fmm", mm)
}

var weekdayJA = [...]string{"日", "月", "火", "水", "木", "金", "土"}

func weekday(t time.Time) string { return weekdayJA[int(t.Weekday())] }

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func windDir(deg float64) string {
	dirs := []string{"北", "北北東", "北東", "東北東", "東", "東南東", "南東", "南南東",
		"南", "南南西", "南西", "西南西", "西", "西北西", "北西", "北北西"}
	i := int(math.Round(deg/22.5)) % 16
	if i < 0 {
		i += 16
	}
	return dirs[i]
}

func sunHM(s string) string {
	if len(s) >= 16 {
		return s[11:16]
	}
	return s
}

// ---------- ヘッダ ----------

func printHeader(w io.Writer, st styler, loc location, fc *apiResponse) {
	var parts []string
	if lbl := loc.label(); lbl != "" {
		parts = append(parts, st.bold(lbl))
	} else {
		parts = append(parts, st.bold(fmt.Sprintf("%.4f, %.4f", loc.Lat, loc.Lon)))
	}
	coord := ""
	if loc.Name != "" {
		coord = fmt.Sprintf("%.4f, %.4f", loc.Lat, loc.Lon)
	}
	if fc.Elevation != 0 {
		coord = strings.TrimSpace(coord + fmt.Sprintf("  標高%.0fm", fc.Elevation))
	}
	if coord != "" {
		parts = append(parts, st.dim(coord))
	}
	fmt.Fprintln(w, strings.Join(parts, "  "))
}

// ---------- テキスト出力 ----------

func renderText(w io.Writer, cfg config, loc location, fc *apiResponse) error {
	unit := "°C"
	if cfg.unit == "f" {
		unit = "°F"
	}
	st := styler{on: cfg.color, unit: unit}
	tz := fc.tz()
	now := time.Now().In(tz)

	printHeader(w, st, loc, fc)

	switch cfg.mode {
	case modeNow:
		return renderNow(w, st, fc, now, tz)
	case modeMinute:
		return renderMinute(w, st, cfg, fc, now, tz)
	case modeDay:
		return renderDaily(w, st, cfg, fc, now, tz)
	default:
		return renderHourly(w, st, cfg, fc, now, tz)
	}
}

type hourRow struct {
	t        time.Time
	code     int
	temp     float64
	apparent float64
	prob     float64
	prec     float64
	isNow    bool
	past     bool
}

func renderHourly(w io.Writer, st styler, cfg config, fc *apiResponse, now time.Time, tz *time.Location) error {
	h := fc.Hourly
	if h == nil || len(h.Time) == 0 {
		return fmt.Errorf("1時間ごとのデータが返りませんでした")
	}

	n := min(len(h.Time), len(h.Temperature), len(h.WeatherCode),
		len(h.PrecipitationProb), len(h.Precipitation))

	var rows []hourRow
	for i := 0; i < n; i++ {
		t, ok := parseLocal(h.Time[i], tz)
		if !ok {
			continue
		}
		// デフォルトは「今日の朝から晩」= 06:00〜21:00。
		if cfg.mode == modeDefault {
			if hr := t.Hour(); hr < 6 || hr > 21 {
				continue
			}
		}
		apparent := h.Temperature[i]
		if i < len(h.ApparentTemperature) {
			apparent = h.ApparentTemperature[i]
		}
		rows = append(rows, hourRow{
			t: t, code: h.WeatherCode[i], temp: h.Temperature[i], apparent: apparent,
			prob: h.PrecipitationProb[i], prec: h.Precipitation[i],
			isNow: sameDay(t, now) && t.Truncate(time.Hour).Equal(now.Truncate(time.Hour)),
			past:  sameDay(t, now) && t.Before(now.Truncate(time.Hour)),
		})
	}
	if len(rows) == 0 {
		return fmt.Errorf("表示できる時刻がありません")
	}

	span := "今日 06:00-21:00"
	if cfg.mode == modeHour {
		if cfg.days == 1 {
			span = "今日 24時間 1時間ごと"
		} else {
			span = fmt.Sprintf("%d日間 1時間ごと", cfg.days)
		}
	}
	fmt.Fprintf(w, "%s  %s\n\n",
		st.bold(now.Format("2006-01-02")+" ("+weekday(now)+")"), st.dim(span))

	labelW := 6
	for _, r := range rows {
		labelW = max(labelW, dispWidth(wmoLabel(r.code)))
	}

	fmt.Fprintf(w, " %s  %s  %s  %s  %s  %s\n",
		st.dim(padRight("時刻", 6)),
		st.dim(padRight("天気", labelW+2)),
		st.dim(padLeft("気温", 7)),
		st.dim(padRight("降水確率", 17)),
		st.dim(padLeft("降水量", 7)),
		st.dim("体感"))

	var prevDay time.Time
	for _, r := range rows {
		if cfg.mode == modeHour && (prevDay.IsZero() || !sameDay(prevDay, r.t)) {
			prevDay = r.t
			fmt.Fprintf(w, " %s\n", st.dim(r.t.Format("01/02 ("+weekday(r.t)+")")))
		}

		label := wmoLabel(r.code)
		line := fmt.Sprintf(" %s  %s  %s  %s  %s  %s",
			padRight(r.t.Format("15:04"), 6),
			padRight(label, labelW+2),
			st.tempCell(r.temp, 7),
			st.probCell(r.prob, 10, 17),
			st.precipCell(r.prec, 7),
			st.dim(padLeft(st.tempStr(r.apparent), 7)),
		)
		switch {
		case r.isNow:
			line = st.wrap("1", line) + "  " + st.cyan("← 現在")
		case r.past:
			line = st.dim(line)
		}
		fmt.Fprintln(w, line)
	}

	fmt.Fprintf(w, "\n %s  %s   %s  %s   %s  %s\n",
		st.dim("気温"), st.tempCell(maxTemp(rows), 0)+st.dim(" / ")+st.tempCell(minTemp(rows), 0),
		st.dim("最大降水確率"), st.wrap(probColorCode(maxProb(rows)), formatProb(maxProb(rows))),
		st.dim("降水量計"), st.precipCell(sumPrecip(rows), 0))
	return nil
}

func maxTemp(rows []hourRow) float64 {
	v := rows[0].temp
	for _, r := range rows[1:] {
		v = math.Max(v, r.temp)
	}
	return v
}

func minTemp(rows []hourRow) float64 {
	v := rows[0].temp
	for _, r := range rows[1:] {
		v = math.Min(v, r.temp)
	}
	return v
}

func maxProb(rows []hourRow) float64 {
	v := rows[0].prob
	for _, r := range rows[1:] {
		v = math.Max(v, r.prob)
	}
	return v
}

func sumPrecip(rows []hourRow) float64 {
	var v float64
	for _, r := range rows {
		v += r.prec
	}
	return v
}

func renderNow(w io.Writer, st styler, fc *apiResponse, now time.Time, tz *time.Location) error {
	c := fc.Current
	if c == nil {
		return fmt.Errorf("現在の実況が返りませんでした")
	}
	t, ok := parseLocal(c.Time, tz)
	if !ok {
		t = now
	}

	slot := "夜"
	switch hr := t.Hour(); {
	case hr >= 5 && hr < 11:
		slot = "朝"
	case hr >= 11 && hr < 16:
		slot = "昼"
	case hr >= 16 && hr < 19:
		slot = "夕"
	}

	fmt.Fprintf(w, "\n  %s  %s  %s\n\n",
		st.bold(t.Format("15:04")), st.dim(slot), st.dim(t.Format("01/02 ("+weekday(t)+")")))

	rows := [][2]string{
		{"天気", st.bold(wmoIcon(c.WeatherCode) + " " + wmoLabel(c.WeatherCode))},
		{"気温", st.tempCell(c.Temperature, 0)},
		{"体感", st.tempCell(c.ApparentTemperature, 0)},
		{"湿度", fmt.Sprintf("%.0f%%", c.Humidity)},
		{"風", fmt.Sprintf("%s %.1fm/s", windDir(c.WindDirection), c.WindSpeed)},
	}
	if c.Precipitation > 0 {
		rows = append(rows, [2]string{"降水", st.bold(formatPrecip(c.Precipitation))})
	}
	for _, r := range rows {
		fmt.Fprintf(w, "   %s  %s\n", st.dim(padRight(r[0], 4)), r[1])
	}
	return nil
}

func renderMinute(w io.Writer, st styler, cfg config, fc *apiResponse, now time.Time, tz *time.Location) error {
	m := fc.Minutely15
	if m == nil || len(m.Time) == 0 {
		return fmt.Errorf("15分ごとのデータが返りませんでした")
	}
	n := min(len(m.Time), len(m.Temperature), len(m.Precipitation), len(m.PrecipitationProb))

	type slot struct {
		t    time.Time
		temp float64
		prob float64
		prec float64
	}
	start := now.Truncate(15 * time.Minute)
	limit := start.Add(time.Duration(cfg.hours) * time.Hour)

	var slots []slot
	for i := 0; i < n; i++ {
		t, ok := parseLocal(m.Time[i], tz)
		if !ok || t.Before(start) || !t.Before(limit) {
			continue
		}
		slots = append(slots, slot{t, m.Temperature[i], m.PrecipitationProb[i], m.Precipitation[i]})
	}
	if len(slots) == 0 {
		return fmt.Errorf("表示できる時刻がありません")
	}

	fmt.Fprintf(w, "%s  %s\n\n",
		st.bold(now.Format("2006-01-02")+" ("+weekday(now)+")"),
		st.dim(fmt.Sprintf("15分ごと / これから%d時間", cfg.hours)))

	for i := 0; i < len(slots); i += 4 {
		var cells []string
		for _, s := range slots[i:min(i+4, len(slots))] {
			cells = append(cells, fmt.Sprintf("%s %s %s %s",
				st.dim(s.t.Format("15:04")),
				padLeft(st.tempStr(s.temp), 6),
				padLeft(formatProb(s.prob), 4),
				st.precipCell(s.prec, 6),
			))
		}
		fmt.Fprintf(w, "  %s\n", strings.Join(cells, " │ "))
	}
	return nil
}

func renderDaily(w io.Writer, st styler, cfg config, fc *apiResponse, now time.Time, tz *time.Location) error {
	d := fc.Daily
	if d == nil || len(d.Time) == 0 {
		return fmt.Errorf("デイリーのデータが返りませんでした")
	}
	n := min(len(d.Time), len(d.WeatherCode), len(d.TempMax), len(d.TempMin),
		len(d.PrecipitationProb), len(d.PrecipitationSum))

	fmt.Fprintf(w, "%s\n\n", st.dim(fmt.Sprintf("%d日間の予報", cfg.days)))

	labelW := 8
	for i := 0; i < n; i++ {
		labelW = max(labelW, dispWidth(wmoLabel(d.WeatherCode[i])))
	}

	fmt.Fprintf(w, " %s  %s  %s  %s  %s  %s\n",
		st.dim(padRight("日付", 12)),
		st.dim(padRight("天気", labelW)),
		st.dim(padLeft("最高/最低", 15)),
		st.dim(padRight("降水確率", 17)),
		st.dim(padLeft("降水量", 7)),
		st.dim("日の出/日の入"))

	for i := 0; i < n; i++ {
		t, ok := parseLocal(d.Time[i], tz)
		if !ok {
			continue
		}
		dateCell := padRight(t.Format("01/02")+" ("+weekday(t)+")", 12)
		if sameDay(t, now) {
			dateCell = st.bold(dateCell)
		}
		sun := ""
		if i < len(d.Sunrise) && i < len(d.Sunset) {
			sun = st.dim(sunHM(d.Sunrise[i]) + " / " + sunHM(d.Sunset[i]))
		}
		temps := padLeft(st.tempStr(d.TempMax[i])+"/"+st.tempStr(d.TempMin[i]), 15)
		fmt.Fprintf(w, " %s  %s  %s  %s  %s  %s\n",
			dateCell,
			padRight(wmoLabel(d.WeatherCode[i]), labelW),
			temps,
			st.probCell(d.PrecipitationProb[i], 10, 17),
			st.precipCell(d.PrecipitationSum[i], 7),
			sun,
		)
	}
	return nil
}

// ---------- JSON 出力 ----------

type jsonLocation struct {
	Name      string  `json:"name,omitempty"`
	Admin     string  `json:"admin,omitempty"`
	Country   string  `json:"country,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation"`
	Timezone  string  `json:"timezone"`
}

type jsonCurrent struct {
	Time        string  `json:"time"`
	Temperature float64 `json:"temperature_2m"`
	Apparent    float64 `json:"apparent_temperature"`
	Humidity    float64 `json:"relative_humidity_2m"`
	WeatherCode int     `json:"weather_code"`
	Weather     string  `json:"weather"`
	Precip      float64 `json:"precipitation"`
	WindSpeed   float64 `json:"wind_speed_10m"`
	WindDir     float64 `json:"wind_direction_10m"`
}

type jsonHourly struct {
	Time        string  `json:"time"`
	Temperature float64 `json:"temperature_2m"`
	Apparent    float64 `json:"apparent_temperature"`
	Prob        float64 `json:"precipitation_probability"`
	Precip      float64 `json:"precipitation"`
	WeatherCode int     `json:"weather_code"`
	Weather     string  `json:"weather"`
}

type jsonDaily struct {
	Date    string  `json:"date"`
	Code    int     `json:"weather_code"`
	Weather string  `json:"weather"`
	TempMax float64 `json:"temperature_2m_max"`
	TempMin float64 `json:"temperature_2m_min"`
	Prob    float64 `json:"precipitation_probability_max"`
	Precip  float64 `json:"precipitation_sum"`
	Sunrise string  `json:"sunrise,omitempty"`
	Sunset  string  `json:"sunset,omitempty"`
}

type jsonMinutely struct {
	Time        string  `json:"time"`
	Temperature float64 `json:"temperature_2m"`
	Prob        float64 `json:"precipitation_probability"`
	Precip      float64 `json:"precipitation"`
}

type jsonOutput struct {
	Location   jsonLocation   `json:"location"`
	Mode       string         `json:"mode"`
	Unit       string         `json:"temperature_unit"`
	Current    *jsonCurrent   `json:"current,omitempty"`
	Hourly     []jsonHourly   `json:"hourly,omitempty"`
	Daily      []jsonDaily    `json:"daily,omitempty"`
	Minutely15 []jsonMinutely `json:"minutely_15,omitempty"`
}

func renderJSON(w io.Writer, cfg config, loc location, fc *apiResponse) error {
	tz := fc.tz()
	now := time.Now().In(tz)

	out := jsonOutput{
		Location: jsonLocation{
			Name: loc.Name, Admin: loc.Admin, Country: loc.Country,
			Latitude: loc.Lat, Longitude: loc.Lon,
			Elevation: fc.Elevation, Timezone: fc.Timezone,
		},
		Mode: string(cfg.mode),
		Unit: map[string]string{"c": "celsius", "f": "fahrenheit"}[cfg.unit],
	}

	if c := fc.Current; c != nil {
		out.Current = &jsonCurrent{
			Time: c.Time, Temperature: round1(c.Temperature), Apparent: round1(c.ApparentTemperature),
			Humidity: c.Humidity, WeatherCode: c.WeatherCode, Weather: wmoLabel(c.WeatherCode),
			Precip: round2(c.Precipitation), WindSpeed: round1(c.WindSpeed), WindDir: c.WindDirection,
		}
	}

	if h := fc.Hourly; h != nil {
		n := min(len(h.Time), len(h.Temperature), len(h.WeatherCode),
			len(h.PrecipitationProb), len(h.Precipitation), len(h.ApparentTemperature))
		for i := 0; i < n; i++ {
			t, ok := parseLocal(h.Time[i], tz)
			if !ok {
				continue
			}
			if cfg.mode == modeDefault {
				if hr := t.Hour(); hr < 6 || hr > 21 {
					continue
				}
			}
			out.Hourly = append(out.Hourly, jsonHourly{
				Time: h.Time[i], Temperature: round1(h.Temperature[i]),
				Apparent: round1(h.ApparentTemperature[i]), Prob: h.PrecipitationProb[i],
				Precip: round2(h.Precipitation[i]), WeatherCode: h.WeatherCode[i],
				Weather: wmoLabel(h.WeatherCode[i]),
			})
		}
	}

	if d := fc.Daily; d != nil {
		n := min(len(d.Time), len(d.WeatherCode), len(d.TempMax), len(d.TempMin),
			len(d.PrecipitationProb), len(d.PrecipitationSum))
		for i := 0; i < n; i++ {
			jd := jsonDaily{
				Date: d.Time[i], Code: d.WeatherCode[i], Weather: wmoLabel(d.WeatherCode[i]),
				TempMax: round1(d.TempMax[i]), TempMin: round1(d.TempMin[i]),
				Prob: d.PrecipitationProb[i], Precip: round2(d.PrecipitationSum[i]),
			}
			if i < len(d.Sunrise) {
				jd.Sunrise = d.Sunrise[i]
			}
			if i < len(d.Sunset) {
				jd.Sunset = d.Sunset[i]
			}
			out.Daily = append(out.Daily, jd)
		}
	}

	if m := fc.Minutely15; m != nil {
		n := min(len(m.Time), len(m.Temperature), len(m.Precipitation), len(m.PrecipitationProb))
		start := now.Truncate(15 * time.Minute)
		limit := start.Add(time.Duration(cfg.hours) * time.Hour)
		for i := 0; i < n; i++ {
			t, ok := parseLocal(m.Time[i], tz)
			if !ok || t.Before(start) || !t.Before(limit) {
				continue
			}
			out.Minutely15 = append(out.Minutely15, jsonMinutely{
				Time: m.Time[i], Temperature: round1(m.Temperature[i]),
				Prob: m.PrecipitationProb[i], Precip: round2(m.Precipitation[i]),
			})
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
