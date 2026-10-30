package main

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCandidateQueries(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		// 漢字のみの都市名はローマ字エイリアスが補われる
		{"大阪", []string{"大阪", "Osaka"}},
		{"京都", []string{"京都", "Kyoto"}},
		{"札幌", []string{"札幌", "Sapporo"}},
		// 接尾辞は落として再検索する
		{"大阪市", []string{"大阪市", "大阪", "Osaka"}},
		{"京都府", []string{"京都府", "京都", "Kyoto"}},
		// 接尾辞込みでエイリアスを持つ語は壊さない
		{"中央区", []string{"中央区", "Chuo"}},
		// エイリアスに無い語はそのまま
		{"上落合", []string{"上落合"}},
		// ローマ字はそのまま通す
		{"Yokohama", []string{"Yokohama"}},
		{"高田馬場", []string{"高田馬場", "Takadanobaba"}},
	}

	for _, tt := range tests {
		got := candidateQueries(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("candidateQueries(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("candidateQueries(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}

func TestCandidateQueriesDedup(t *testing.T) {
	// 「東京」は接尾辞「都」を落としても同じなので重複しない
	got := candidateQueries("東京")
	seen := map[string]bool{}
	for _, g := range got {
		if seen[g] {
			t.Fatalf("重複した候補: %v", got)
		}
		seen[g] = true
	}
}

// 「横浜」で青森県の横浜(人口4千)ではなく横浜市が選ばれること。
func TestScoreResultPrefersRepresentativePlace(t *testing.T) {
	yokohamaCity := geocodeResult{
		Name: "横浜市", Admin1: "神奈川県", FeatureCode: "PPLA",
		Population: 3777491, Latitude: 35.43333,
	}
	aomoriYokohama := geocodeResult{
		Name: "横浜", Admin1: "青森県", FeatureCode: "PPL",
		Population: 4412, Latitude: 41.08333,
	}

	cityScore := scoreResult(yokohamaCity, "横浜")
	villageScore := scoreResult(aomoriYokohama, "横浜")

	if cityScore <= villageScore {
		t.Errorf("横浜市 (%.1f) が青森の横浜 (%.1f) を上回っていない", cityScore, villageScore)
	}
}

// 人口が同じくらいなら県庁所在地が勝つこと。
func TestScoreResultFeatureCode(t *testing.T) {
	a := geocodeResult{Name: "宮崎", FeatureCode: "PPLA", Population: 400000}
	b := geocodeResult{Name: "宮崎", FeatureCode: "PPL", Population: 400000}
	if scoreResult(a, "宮崎") <= scoreResult(b, "宮崎") {
		t.Error("PPLA が PPL より優先されていない")
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"abc", 3},
		{"曇り", 4},
		{"晴れ時々曇り", 12},
		{"16.2°C", 6},
		{"█░░░░", 5},
		{"", 0},
	}
	for _, tt := range tests {
		if got := dispWidth(tt.in); got != tt.want {
			t.Errorf("dispWidth(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestPad(t *testing.T) {
	if got := padRight("曇り", 8); got != "曇り    " {
		t.Errorf("padRight = %q", got)
	}
	if got := padLeft("16.2°C", 10); got != "    16.2°C" {
		t.Errorf("padLeft = %q", got)
	}
	// 幅を超えるときは切らない
	if got := padRight("晴れ時々曇り", 4); got != "晴れ時々曇り" {
		t.Errorf("padRight は切り詰めてはいけない: %q", got)
	}
}

func TestPrecipBar(t *testing.T) {
	tests := []struct {
		p, width int
		want     string
	}{
		{0, 10, "░░░░░░░░░░"},
		{100, 10, "██████████"},
		{50, 10, "█████░░░░░"},
		{150, 10, "██████████"}, // 範囲外は丸める
		{-5, 10, "░░░░░░░░░░"},
	}
	for _, tt := range tests {
		got := precipBar(float64(tt.p), tt.width)
		if got != tt.want {
			t.Errorf("precipBar(%d, %d) = %q, want %q", tt.p, tt.width, got, tt.want)
		}
		if w := dispWidth(got); w != tt.width {
			t.Errorf("precipBar の幅が %d になっている: %d", w, tt.width)
		}
	}
}

func TestPrecipColorsAreBlue(t *testing.T) {
	blue := "38;5;"
	for _, p := range []float64{0, 10, 20, 49, 50, 79, 80, 100} {
		code := probColorCode(p)
		if code == "2" {
			continue
		}
		if !strings.HasPrefix(code, blue) {
			t.Fatalf("probColorCode(%.0f) = %q, 色コード形式が想定外", p, code)
		}
		n, err := strconv.Atoi(strings.TrimPrefix(code, blue))
		if err != nil {
			t.Fatalf("probColorCode(%.0f) = %q を解釈できない", p, code)
		}
		// 256 色は 16-51 が青系。赤系（196-231）は使わない。
		if n < 16 || n > 51 {
			t.Errorf("probColorCode(%.0f) = %q は青系ではない", p, code)
		}
	}
	if got := precipColorCode(0); got != "2" {
		t.Errorf("precipColorCode(0) = %q, 降っていないときは dim にしたい", got)
	}
	if got := precipColorCode(1.2); got != "38;5;39" {
		t.Errorf("precipColorCode(1.2) = %q, 降水量は青にしたい", got)
	}
}

func TestWmoLabel(t *testing.T) {
	tests := map[int]string{
		0:  "快晴",
		3:  "曇り",
		61: "弱い雨",
		95: "雷雨",
		99: "雷雨と強い雹",
		-1: "不明",
		42: "不明",
	}
	for code, want := range tests {
		if got := wmoLabel(code); got != want {
			t.Errorf("wmoLabel(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestFormatPrecip(t *testing.T) {
	tests := map[float64]string{
		0:     "0.0mm",
		0.04:  "0.0mm",
		0.1:   "0.1mm",
		9.94:  "9.9mm",
		16.0:  "16mm",
		123.4: "123mm",
	}
	for in, want := range tests {
		if got := formatPrecip(in); got != want {
			t.Errorf("formatPrecip(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestWindDir(t *testing.T) {
	tests := map[float64]string{
		0: "北", 90: "東", 180: "南", 270: "西", 45: "北東", 350: "北",
	}
	for in, want := range tests {
		if got := windDir(in); got != want {
			t.Errorf("windDir(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildURL(t *testing.T) {
	loc := location{Lat: 35.7131, Lon: 139.7001}

	t.Run("default", func(t *testing.T) {
		u := buildURL(config{mode: modeDefault, days: 1, unit: "c"}, loc)
		for _, want := range []string{
			"latitude=35.7131", "longitude=139.7001", "timezone=auto",
			"precipitation_probability", "weather_code", "forecast_days=1",
			"wind_speed_unit=ms",
		} {
			if !strings.Contains(u, want) {
				t.Errorf("URL に %q が無い: %s", want, u)
			}
		}
		if strings.Contains(u, "temperature_unit=fahrenheit") {
			t.Error("摂氏指定なのに fahrenheit が入っている")
		}
	})

	t.Run("fahrenheit", func(t *testing.T) {
		u := buildURL(config{mode: modeDefault, days: 1, unit: "f"}, loc)
		if !strings.Contains(u, "temperature_unit=fahrenheit") {
			t.Errorf("fahrenheit が無い: %s", u)
		}
	})

	t.Run("now", func(t *testing.T) {
		u := buildURL(config{mode: modeNow, days: 1, unit: "c"}, loc)
		if !strings.Contains(u, "current=") {
			t.Errorf("current が無い: %s", u)
		}
		if strings.Contains(u, "hourly=") {
			t.Errorf("now モードで hourly を要求している: %s", u)
		}
	})

	t.Run("week", func(t *testing.T) {
		u := buildURL(config{mode: modeDay, days: 7, unit: "c"}, loc)
		if !strings.Contains(u, "forecast_days=7") {
			t.Errorf("forecast_days=7 が無い: %s", u)
		}
		if !strings.Contains(u, "sunrise") {
			t.Errorf("sunrise が無い: %s", u)
		}
	})

	t.Run("minute", func(t *testing.T) {
		u := buildURL(config{mode: modeMinute, days: 1, unit: "c"}, loc)
		if !strings.Contains(u, "minutely_15=") {
			t.Errorf("minutely_15 が無い: %s", u)
		}
	})
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		check   func(*testing.T, config)
	}{
		{
			name: "引数なしはデフォルト",
			args: nil,
			check: func(t *testing.T, c config) {
				if c.mode != modeDefault {
					t.Errorf("mode = %q", c.mode)
				}
				if c.place != "" {
					t.Errorf("place = %q", c.place)
				}
			},
		},
		{
			name: "位置引数が地点になる",
			args: []string{"高田馬場"},
			check: func(t *testing.T, c config) {
				if c.place != "高田馬場" {
					t.Errorf("place = %q", c.place)
				}
			},
		},
		{
			name: "スペースを含む地点名",
			args: []string{"Minato", "City"},
			check: func(t *testing.T, c config) {
				if c.place != "Minato City" {
					t.Errorf("place = %q", c.place)
				}
			},
		},
		{
			name: "week は day の7日版になる",
			args: []string{"-week"},
			check: func(t *testing.T, c config) {
				if c.mode != modeDay || c.days != 7 {
					t.Errorf("mode=%q days=%d", c.mode, c.days)
				}
			},
		},
		{
			name: "座標指定",
			args: []string{"-lat", "35.68", "-lon", "139.76"},
			check: func(t *testing.T, c config) {
				if !c.hasLat || !c.hasLon || c.lat != 35.68 || c.lon != 139.76 {
					t.Errorf("%+v", c)
				}
			},
		},
		{name: "lat だけはエラー", args: []string{"-lat", "35.68"}, wantErr: true},
		{name: "lat 範囲外", args: []string{"-lat", "95", "-lon", "139"}, wantErr: true},
		{name: "モード複数はエラー", args: []string{"-now", "-week"}, wantErr: true},
		{name: "unit 不正", args: []string{"-unit", "k"}, wantErr: true},
		{name: "days 範囲外", args: []string{"-days", "99"}, wantErr: true},
		{name: "hours 範囲外", args: []string{"-hours", "0"}, wantErr: true},
		{name: "color 不正", args: []string{"-color", "rainbow"}, wantErr: true},
		{name: "未知のフラグ", args: []string{"-bogus"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseArgs(tt.args, &strings.Builder{})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("エラーになるはずが nil: %+v", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestLocationLabel(t *testing.T) {
	tests := []struct {
		loc  location
		want string
	}{
		{location{Name: "高田馬場", Admin: "東京都", Country: "日本"}, "高田馬場 東京都 日本"},
		// 名前と admin が同じなら畳む
		{location{Name: "東京都", Admin: "東京都", Country: "日本"}, "東京都 日本"},
		{location{Name: "大阪市", Admin: "大阪府"}, "大阪市 大阪府"},
		{location{Lat: 35.7, Lon: 139.7}, ""},
	}
	for _, tt := range tests {
		if got := tt.loc.label(); got != tt.want {
			t.Errorf("label() = %q, want %q", got, tt.want)
		}
	}
}

func TestParseLocal(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip("tzdata が無い:", err)
	}
	got, ok := parseLocal("2026-10-27T14:00", tz)
	if !ok {
		t.Fatal("パースに失敗")
	}
	if got.Hour() != 14 || got.Day() != 27 {
		t.Errorf("got %v", got)
	}
	if _, ok := parseLocal("bogus", tz); ok {
		t.Error("不正な入力が通ってしまった")
	}
}

func TestSunHM(t *testing.T) {
	if got := sunHM("2026-10-27T06:02"); got != "06:02" {
		t.Errorf("sunHM = %q", got)
	}
	if got := sunHM("短い"); got != "短い" {
		t.Errorf("sunHM = %q", got)
	}
}
