// Command tenki は Open-Meteo（APIキー不要）から天気を取得する CLI。
//
// 引数なしで「今日の 6:00〜21:00 を1時間ごと」を出す。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const version = "0.1.0"

// モードは「同時に1つだけ」選べる出力の種類。
type mode string

const (
	modeDefault mode = "default" // 今日の 6:00-21:00 を1時間ごと
	modeNow     mode = "now"     // 現在の実況
	modeHour    mode = "hour"    // 1時間ごと（全時間）
	modeMinute  mode = "minute"  // 15分ごと
	modeDay     mode = "day"     // デイリー
	modeWeek    mode = "week"    // 7日間デイリー
)

type config struct {
	mode mode

	place    string
	lat, lon float64
	hasLat   bool
	hasLon   bool

	days  int // -hour / -day の日数
	hours int // -minute の表示幅（時間）

	json  bool
	unit  string // "c" or "f"
	color bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "tenki: "+err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := parseArgs(args, stderr)
	if err != nil {
		return err
	}

	loc, err := resolveLocation(ctx, cfg)
	if err != nil {
		return err
	}

	fc, err := fetchForecast(ctx, cfg, loc)
	if err != nil {
		return err
	}

	if cfg.json {
		return renderJSON(stdout, cfg, loc, fc)
	}
	return renderText(stdout, cfg, loc, fc)
}

func parseArgs(args []string, stderr io.Writer) (config, error) {
	cfg := config{days: 1, hours: 3, unit: "c"}

	fs := flag.NewFlagSet("tenki", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usageText) }

	var (
		fNow    = fs.Bool("now", false, "現在の実況だけ表示")
		fHour   = fs.Bool("hour", false, "1時間ごとの予報")
		fMinute = fs.Bool("minute", false, "15分ごとの予報")
		fDay    = fs.Bool("day", false, "デイリー予報")
		fWeek   = fs.Bool("week", false, "7日間のデイリー予報")

		fPlace = fs.String("place", "", "地点名（例: 高田馬場, Shinjuku, Osaka）")
		fLat   = fs.Float64("lat", 0, "緯度（-lon と併用）")
		fLon   = fs.Float64("lon", 0, "経度（-lat と併用）")

		fJSON  = fs.Bool("json", false, "JSON で出力")
		fUnit  = fs.String("unit", "c", "気温の単位: c または f")
		fColor = fs.String("color", "auto", "色付け: auto / always / never")
	)
	fs.IntVar(&cfg.days, "days", 1, "-hour / -day の日数")
	fs.IntVar(&cfg.hours, "hours", 3, "-minute で何時間先まで出すか")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if rest := fs.Args(); len(rest) > 0 {
		cfg.place = strings.Join(rest, " ")
	}
	if *fPlace != "" {
		cfg.place = *fPlace
	}

	// モードは1つだけ。複数指定はユーザーの意図が曖昧なのでエラーにする。
	var selected []mode
	for _, m := range []struct {
		on bool
		m  mode
	}{
		{*fNow, modeNow}, {*fMinute, modeMinute}, {*fHour, modeHour},
		{*fWeek, modeWeek}, {*fDay, modeDay},
	} {
		if m.on {
			selected = append(selected, m.m)
		}
	}
	switch len(selected) {
	case 0:
		cfg.mode = modeDefault
	case 1:
		cfg.mode = selected[0]
	default:
		names := make([]string, len(selected))
		for i, m := range selected {
			names[i] = "-" + string(m)
		}
		return cfg, fmt.Errorf("モードは1つだけ指定してください（%s）", strings.Join(names, ", "))
	}
	// -week は「-day の7日版」なので同じ経路に寄せる。
	if cfg.mode == modeWeek {
		cfg.mode = modeDay
		cfg.days = 7
	}

	latSet, lonSet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "lat":
			latSet = true
		case "lon":
			lonSet = true
		}
	})
	if latSet != lonSet {
		return cfg, errors.New("-lat と -lon は両方指定してください")
	}
	cfg.lat, cfg.lon, cfg.hasLat, cfg.hasLon = *fLat, *fLon, latSet, lonSet
	if cfg.hasLat && (cfg.lat < -90 || cfg.lat > 90) {
		return cfg, fmt.Errorf("-lat は -90〜90 の範囲で指定してください: %v", cfg.lat)
	}
	if cfg.hasLon && (cfg.lon < -180 || cfg.lon > 180) {
		return cfg, fmt.Errorf("-lon は -180〜180 の範囲で指定してください: %v", cfg.lon)
	}

	if cfg.days < 1 || cfg.days > 16 {
		return cfg, fmt.Errorf("-days は 1〜16 で指定してください: %d", cfg.days)
	}
	if cfg.hours < 1 || cfg.hours > 24 {
		return cfg, fmt.Errorf("-hours は 1〜24 で指定してください: %d", cfg.hours)
	}

	switch strings.ToLower(*fUnit) {
	case "c", "celsius":
		cfg.unit = "c"
	case "f", "fahrenheit":
		cfg.unit = "f"
	default:
		return cfg, fmt.Errorf("-unit は c か f で指定してください: %q", *fUnit)
	}

	cfg.json = *fJSON
	switch strings.ToLower(*fColor) {
	case "auto":
		cfg.color = colorSupported(stdoutIsTerminal())
	case "always", "yes", "on":
		cfg.color = true
	case "never", "no", "off":
		cfg.color = false
	default:
		return cfg, fmt.Errorf("-color は auto / always / never で指定してください: %q", *fColor)
	}

	return cfg, nil
}

// stdoutIsTerminal は出力先が端末かどうかを見る（色付けの auto 判定用）。
func stdoutIsTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func colorSupported(isTTY bool) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	return isTTY
}

// location は解決済みの地点。
type location struct {
	Name     string
	Admin    string
	Country  string
	Lat      float64
	Lon      float64
	Elev     float64
	Resolved bool // ジオコーディングで引いたか（座標直指定なら false）
}

func (l location) label() string {
	parts := nonEmpty(l.Name, l.Admin, l.Country)
	// 「東京都 東京都 日本」のような重複を畳む
	out := parts[:0]
	seen := map[string]bool{}
	for _, p := range parts {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, " ")
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func resolveLocation(ctx context.Context, cfg config) (location, error) {
	if cfg.hasLat {
		return location{Lat: cfg.lat, Lon: cfg.lon}, nil
	}

	name := cfg.place
	if name == "" {
		name = os.Getenv("TENKI_PLACE")
	}
	if name == "" {
		name = "Tokyo"
	}
	return geocode(ctx, name)
}

const usageText = `tenki — コマンドライン天気 (Open-Meteo / APIキー不要)

使い方:
  tenki [フラグ] [地点]

地点:
  省略時は Tokyo。日本語・英語どちらでも可 (例: 高田馬場, Shinjuku, Osaka)。
  -lat/-lon で座標を直接渡すとジオコーディングを飛ばす。
  TENKI_PLACE 環境変数でも既定地点を指定できる。

モード (同時に1つだけ / 省略時はデフォルト):
  (なし)   今日の 06:00〜21:00 を1時間ごと
  -now     現在の実況だけ
  -hour    1時間ごと (既定24時間 / -days N で延長)
  -minute  15分ごと (既定3時間先まで / -hours N で変更)
  -day     デイリー (既定1日 / -days N)
  -week    7日間のデイリー

フラグ:
  -place NAME   地点名
  -lat F -lon F 座標を直接指定
  -days N       -hour / -day の日数 (1-16)
  -hours N      -minute で何時間先まで (1-24)
  -json         JSON で出力
  -unit c|f     気温の単位
  -color MODE   auto / always / never (NO_COLOR も尊重)
  -version      バージョン表示

例:
  tenki
  tenki 高田馬場
  tenki -now 新宿
  tenki -hour -days 2 大阪
  tenki -week -lat 35.7131 -lon 139.7001
  tenki -minute -hours 6 京都
`
