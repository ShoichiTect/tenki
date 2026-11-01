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

// fixtureRolling は「今」をまたぐ 30 時間分のデータを作る（27日12:00 から）。
func fixtureRolling(tz *time.Location) (*apiResponse, time.Time) {
	now := time.Date(2026, 10, 27, 14, 30, 0, 0, tz)
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, tz)

	h := &apiHourly{}
	for i := 0; i < 30; i++ {
		t := base.Add(time.Duration(i) * time.Hour)
		temp := 15 + float64(i)/10 // 15.0 から 17.9 まで 0.1 ずつ
		h.Time = append(h.Time, t.Format("2006-01-02T15:04"))
		h.Temperature = append(h.Temperature, temp)
		h.ApparentTemperature = append(h.ApparentTemperature, temp)
		h.PrecipitationProb = append(h.PrecipitationProb, float64(i))
		h.Precipitation = append(h.Precipitation, 0)
		h.WeatherCode = append(h.WeatherCode, 1)
	}
	return &apiResponse{
		Latitude: 35.7131, Longitude: 139.7001, Elevation: 30,
		Timezone: "Asia/Tokyo", UTCOffsetSeconds: 32400, Hourly: h,
	}, now
}

// デフォルトモードは「今の時刻（時間切り捨て）から24時間」に絞られること。
func TestRenderHourlyDefaultWindow(t *testing.T) {
	tz := jst(t)
	fc, now := fixtureRolling(tz)

	var b strings.Builder
	err := renderHourly(&b, styler{on: false, unit: "°C"}, config{mode: modeDefault, days: 1}, fc, now, tz)
	if err != nil {
		t.Fatal(err)
	}
	out := b.String()

	// 14:30 の時間切り捨て = 14:00 から 24 時間（翌日 14:00 は含まない）
	for _, want := range []string{"14:00", "21:00", "23:00", "10/28 (水)", "これから24時間", "← 現在"} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q が無い:\n%s", want, out)
		}
	}
	// 窓の外は出さない（境界の 0.1 刻みの気温で確かめる）
	for _, unwanted := range []string{"15.0°C", "15.1°C", "17.6°C", "17.7°C"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("出力に %q が出ている:\n%s", unwanted, out)
		}
	}
	// サマリは窓の中だけ（15.2〜17.5 / 最大降水確率 25%）
	for _, want := range []string{"17.5°C", "15.2°C", "25%"} {
		if !strings.Contains(out, want) {
			t.Errorf("サマリに %q が無い:\n%s", want, out)
		}
	}
}

// -hours N でデフォルトの窓を縮められること。
func TestRenderHourlyDefaultWindowHours(t *testing.T) {
	tz := jst(t)
	fc, now := fixtureRolling(tz)

	var b strings.Builder
	if err := renderHourly(&b, styler{on: false, unit: "°C"},
		config{mode: modeDefault, days: 1, hours: 1, window: 1}, fc, now, tz); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	for _, want := range []string{"14:00", "これから1時間"} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q が無い:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"15.3°C", "10/28"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("出力に %q が出ている:\n%s", unwanted, out)
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
	if err := renderJSONAt(&b, config{mode: modeDefault, days: 1, unit: "c"},
		location{Name: "高田馬場", Admin: "東京都", Lat: 35.7131, Lon: 139.7001}, fc,
		time.Date(2026, 10, 27, 14, 30, 0, 0, tz)); err != nil {
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
	// 14:30 時点の窓（14:00〜翌日14:00）より前の時刻は JSON にも入らない
	if strings.Contains(out, `"2026-10-27T07:00"`) {
		t.Errorf("JSON に窓の外の時刻が入っている:\n%s", out)
	}
	if !strings.Contains(out, `"2026-10-27T21:00"`) {
		t.Errorf("JSON に窓の中の時刻が無い:\n%s", out)
	}
}
