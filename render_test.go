package main

import (
	"strings"
	"testing"
	"time"
)

func jst(t *testing.T) *time.Location {
	t.Helper()
	l, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip("tzdata が無い:", err)
	}
	return l
}

func fixtureHourly(tz *time.Location) *apiResponse {
	return &apiResponse{
		Latitude: 35.7131, Longitude: 139.7001, Elevation: 30,
		Timezone: "Asia/Tokyo", UTCOffsetSeconds: 32400,
		Hourly: &apiHourly{
			Time: []string{
				"2026-10-27T05:00", "2026-10-27T06:00", "2026-10-27T07:00",
				"2026-10-27T14:00", "2026-10-27T21:00", "2026-10-27T22:00",
			},
			Temperature:         []float64{15.9, 16.1, 16.0, 20.1, 17.1, 16.5},
			ApparentTemperature: []float64{16.0, 17.0, 16.9, 22.0, 18.8, 17.0},
			PrecipitationProb:   []float64{60, 53, 47, 5, 14, 90},
			Precipitation:       []float64{0, 0, 0, 0, 0, 1.2},
			WeatherCode:         []int{3, 3, 3, 1, 0, 63},
		},
	}
}

// デフォルトモードは 06:00〜21:00 に絞られること。
func TestRenderHourlyDefaultWindow(t *testing.T) {
	tz := jst(t)
	fc := fixtureHourly(tz)
	now := time.Date(2026, 10, 27, 14, 30, 0, 0, tz)

	var b strings.Builder
	err := renderHourly(&b, styler{on: false, unit: "°C"}, config{mode: modeDefault, days: 1}, fc, now, tz)
	if err != nil {
		t.Fatal(err)
	}
	out := b.String()

	for _, want := range []string{"06:00", "07:00", "14:00", "21:00", "晴れ", "快晴", "曇り"} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q が無い:\n%s", want, out)
		}
	}
	// 窓の外は出さない
	for _, unwanted := range []string{"05:00", "22:00"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("出力に %q が出ている:\n%s", unwanted, out)
		}
	}
	// 現在の時刻に印が付く
	if !strings.Contains(out, "← 現在") {
		t.Errorf("現在の印が無い:\n%s", out)
	}
	// サマリが入る（最大 20.1 / 最小 16.0 / 最大降水確率 53%）
	// 22:00 の 90% は窓の外なので、サマリにも反映されない
	for _, want := range []string{"20.1°C", "16.0°C", "53%"} {
		if !strings.Contains(out, want) {
			t.Errorf("サマリに %q が無い:\n%s", want, out)
		}
	}
}

// 色を付けたとき、桁揃えのための空白が ANSI の内側に入ること
// （見た目の幅が変わらないこと）を確かめる。
func TestRenderHourlyColorKeepsColumns(t *testing.T) {
	tz := jst(t)
	fc := fixtureHourly(tz)
	now := time.Date(2026, 10, 27, 14, 30, 0, 0, tz)

	var plain, colored strings.Builder
	cfg := config{mode: modeDefault, days: 1}
	if err := renderHourly(&plain, styler{on: false, unit: "°C"}, cfg, fc, now, tz); err != nil {
		t.Fatal(err)
	}
	if err := renderHourly(&colored, styler{on: true, unit: "°C"}, cfg, fc, now, tz); err != nil {
		t.Fatal(err)
	}

	plainLines := strings.Split(strings.TrimRight(plain.String(), "\n"), "\n")
	coloredLines := strings.Split(strings.TrimRight(colored.String(), "\n"), "\n")
	if len(plainLines) != len(coloredLines) {
		t.Fatalf("行数が違う: %d vs %d", len(plainLines), len(coloredLines))
	}
	for i := range plainLines {
		a := dispWidth(plainLines[i])
		b := dispWidth(stripANSI(coloredLines[i]))
		if a != b {
			t.Errorf("%d 行目の表示幅が違う: 色なし %d / 色あり %d\n%s\n%s",
				i+1, a, b, plainLines[i], stripANSI(coloredLines[i]))
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			// ESC [ ... m
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && s[j] != 'm' {
					j++
				}
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestRenderMinuteWindow(t *testing.T) {
	tz := jst(t)
	fc := &apiResponse{
		Timezone: "Asia/Tokyo", UTCOffsetSeconds: 32400,
		Minutely15: &apiMinutely15{
			Time: []string{
				"2026-10-27T14:00", "2026-10-27T14:15", "2026-10-27T14:30",
				"2026-10-27T14:45", "2026-10-27T15:00", "2026-10-27T18:00",
			},
			Temperature:       []float64{20.5, 20.3, 20.2, 20.1, 20.0, 18.0},
			Precipitation:     []float64{0, 0, 0.1, 0.3, 0.5, 0},
			PrecipitationProb: []float64{10, 20, 40, 70, 80, 0},
			WeatherCode:       []int{1, 1, 61, 63, 63, 0},
		},
	}
	now := time.Date(2026, 10, 27, 14, 40, 0, 0, tz)

	var b strings.Builder
	if err := renderMinute(&b, styler{on: false, unit: "°C"}, config{mode: modeMinute, hours: 1}, fc, now, tz); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	// 14:40 の 15 分切り捨て = 14:30 から 1 時間
	for _, want := range []string{"14:30", "14:45", "15:00"} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q が無い:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"14:00", "14:15", "18:00"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("出力に %q が出ている:\n%s", unwanted, out)
		}
	}
}

func TestRenderDaily(t *testing.T) {
	tz := jst(t)
	fc := &apiResponse{
		Timezone: "Asia/Tokyo", UTCOffsetSeconds: 32400,
		Daily: &apiDaily{
			Time:              []string{"2026-10-27", "2026-10-28"},
			WeatherCode:       []int{61, 3},
			TempMax:           []float64{20.2, 18.0},
			TempMin:           []float64{16.0, 12.5},
			PrecipitationProb: []float64{90, 30},
			PrecipitationSum:  []float64{3.2, 0},
			Sunrise:           []string{"2026-10-27T06:00", "2026-10-28T06:01"},
			Sunset:            []string{"2026-10-27T16:52", "2026-10-28T16:51"},
		},
	}
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, tz)

	var b strings.Builder
	if err := renderDaily(&b, styler{on: false, unit: "°C"}, config{mode: modeDay, days: 2}, fc, now, tz); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	for _, want := range []string{"10/27 (火)", "10/28 (水)", "弱い雨", "曇り", "20.2°C", "3.2mm", "06:00 / 16:52"} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q が無い:\n%s", want, out)
		}
	}
	// 今日の行は太字にするが色が無効なら素のまま
	if strings.Contains(out, "\x1b[") {
		t.Errorf("色なし指定なのに ANSI が入っている:\n%s", out)
	}
}

func TestRenderJSONShape(t *testing.T) {
	tz := jst(t)
	fc := fixtureHourly(tz)

	var b strings.Builder
	if err := renderJSON(&b, config{mode: modeDefault, days: 1, unit: "c"},
		location{Name: "高田馬場", Admin: "東京都", Lat: 35.7131, Lon: 139.7001}, fc); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	for _, want := range []string{
		`"name": "高田馬場"`, `"mode": "default"`, `"temperature_unit": "celsius"`,
		`"hourly"`, `"weather_code"`, `"precipitation_probability"`, `"timezone": "Asia/Tokyo"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON に %q が無い:\n%s", want, out)
		}
	}
	// 05:00 / 22:00 は窓の外なので JSON にも入らない
	for _, unwanted := range []string{"2026-10-27T05:00", "2026-10-27T22:00"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("JSON に %q が入っている:\n%s", unwanted, out)
		}
	}
}
