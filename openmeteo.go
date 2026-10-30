package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "time/tzdata" // tzdata が無い環境でも timezone=auto を解釈できるようにする
)

const (
	forecastEndpoint = "https://api.open-meteo.com/v1/forecast"
	userAgent        = "tenki/" + version + " (+https://open-meteo.com)"
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

// ---- API レスポンス ----

type apiResponse struct {
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	Elevation        float64 `json:"elevation"`
	Timezone         string  `json:"timezone"`
	UTCOffsetSeconds int     `json:"utc_offset_seconds"`

	Current    *apiCurrent    `json:"current"`
	Hourly     *apiHourly     `json:"hourly"`
	Daily      *apiDaily      `json:"daily"`
	Minutely15 *apiMinutely15 `json:"minutely_15"`

	Error  bool   `json:"error"`
	Reason string `json:"reason"`
}

type apiCurrent struct {
	Time                string  `json:"time"`
	Temperature         float64 `json:"temperature_2m"`
	ApparentTemperature float64 `json:"apparent_temperature"`
	Humidity            float64 `json:"relative_humidity_2m"`
	WeatherCode         int     `json:"weather_code"`
	Precipitation       float64 `json:"precipitation"`
	WindSpeed           float64 `json:"wind_speed_10m"`
	WindDirection       float64 `json:"wind_direction_10m"`
	IsDay               int     `json:"is_day"`
}

type apiHourly struct {
	Time                []string  `json:"time"`
	Temperature         []float64 `json:"temperature_2m"`
	ApparentTemperature []float64 `json:"apparent_temperature"`
	PrecipitationProb   []float64 `json:"precipitation_probability"`
	Precipitation       []float64 `json:"precipitation"`
	WeatherCode         []int     `json:"weather_code"`
}

type apiDaily struct {
	Time              []string  `json:"time"`
	WeatherCode       []int     `json:"weather_code"`
	TempMax           []float64 `json:"temperature_2m_max"`
	TempMin           []float64 `json:"temperature_2m_min"`
	PrecipitationProb []float64 `json:"precipitation_probability_max"`
	PrecipitationSum  []float64 `json:"precipitation_sum"`
	Sunrise           []string  `json:"sunrise"`
	Sunset            []string  `json:"sunset"`
}

type apiMinutely15 struct {
	Time              []string  `json:"time"`
	Temperature       []float64 `json:"temperature_2m"`
	Precipitation     []float64 `json:"precipitation"`
	PrecipitationProb []float64 `json:"precipitation_probability"`
	WeatherCode       []int     `json:"weather_code"`
}

// ---- リクエスト ----

func buildURL(cfg config, loc location) string {
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(loc.Lat, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(loc.Lon, 'f', 4, 64))
	q.Set("timezone", "auto")

	if cfg.unit == "f" {
		q.Set("temperature_unit", "fahrenheit")
	}
	q.Set("wind_speed_unit", "ms")

	switch cfg.mode {
	case modeNow:
		q.Set("current", strings.Join([]string{
			"temperature_2m", "apparent_temperature", "relative_humidity_2m",
			"weather_code", "precipitation", "wind_speed_10m",
			"wind_direction_10m", "is_day",
		}, ","))
	case modeMinute:
		q.Set("minutely_15", strings.Join([]string{
			"temperature_2m", "precipitation", "precipitation_probability", "weather_code",
		}, ","))
		q.Set("forecast_days", "1")
	case modeDay:
		q.Set("daily", strings.Join([]string{
			"weather_code", "temperature_2m_max", "temperature_2m_min",
			"precipitation_probability_max", "precipitation_sum", "sunrise", "sunset",
		}, ","))
		q.Set("forecast_days", strconv.Itoa(cfg.days))
	default: // modeDefault, modeHour
		q.Set("hourly", strings.Join([]string{
			"temperature_2m", "apparent_temperature", "precipitation_probability",
			"precipitation", "weather_code",
		}, ","))
		q.Set("forecast_days", strconv.Itoa(cfg.days))
	}

	return forecastEndpoint + "?" + q.Encode()
}

func fetchForecast(ctx context.Context, cfg config, loc location) (*apiResponse, error) {
	endpoint := buildURL(cfg, loc)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Open-Meteo に接続できません: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("応答の読み取りに失敗: %w", err)
	}

	var out apiResponse
	// Open-Meteo は 400 でも JSON 本文に reason を返すので、先にデコードする。
	if err := json.Unmarshal(body, &out); err != nil {
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d (%s)", resp.StatusCode, strings.TrimSpace(truncate(string(body), 200)))
		}
		return nil, fmt.Errorf("応答を解析できません: %w", err)
	}
	if out.Error {
		return nil, fmt.Errorf("Open-Meteo: %s", out.Reason)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return &out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// tz は API が返した timezone 名を読み、失敗したら utc_offset_seconds で固定オフセットにする。
func (r *apiResponse) tz() *time.Location {
	if r.Timezone != "" {
		if l, err := time.LoadLocation(r.Timezone); err == nil {
			return l
		}
	}
	return time.FixedZone("", r.UTCOffsetSeconds)
}

// parseLocal は "2006-01-02T15:04" 形式の API 時刻をロケーション付きで解釈する。
func parseLocal(s string, loc *time.Location) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
