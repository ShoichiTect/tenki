package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const geocodeEndpoint = "https://geocoding-api.open-meteo.com/v1/search"

// 座標は変わらないのでキャッシュは永続。TTL なし。
type geoCache struct {
	mu   sync.Mutex
	path string
	data map[string]location
}

type geocodeResult struct {
	Name        string  `json:"name"`
	Admin1      string  `json:"admin1"`
	Country     string  `json:"country"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Elevation   float64 `json:"elevation"`
	FeatureCode string  `json:"feature_code"`
	Population  float64 `json:"population"`
}

type geocodeResponse struct {
	Results []geocodeResult `json:"results"`
	Error   bool            `json:"error"`
	Reason  string          `json:"reason"`
}

func geocode(ctx context.Context, query string) (location, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return location{}, fmt.Errorf("地点名が空です")
	}

	cache := loadGeoCache()
	key := strings.ToLower(q)
	if loc, ok := cache.get(key); ok {
		return loc, nil
	}

	// 「大阪」→「Osaka」のように、表記を変えながら候補を集めて一番それらしい点を採る。
	var all []geocodeResult
	for _, cq := range candidateQueries(q) {
		results, err := searchPlaces(ctx, cq)
		if err != nil {
			return location{}, err
		}
		all = append(all, results...)
	}
	if len(all) == 0 {
		return location{}, fmt.Errorf("%q が見つかりません。別の表記か -lat/-lon を試してください", q)
	}

	// 同じ地点が複数クエリから返るので座標で重複排除する。
	seen := map[string]bool{}
	uniq := all[:0]
	for _, r := range all {
		k := fmt.Sprintf("%.3f,%.3f", r.Latitude, r.Longitude)
		if seen[k] {
			continue
		}
		seen[k] = true
		uniq = append(uniq, r)
	}

	sort.SliceStable(uniq, func(i, j int) bool {
		return scoreResult(uniq[i], q) > scoreResult(uniq[j], q)
	})

	best := uniq[0]
	loc := location{
		Name: best.Name, Admin: best.Admin1, Country: best.Country,
		Lat: best.Latitude, Lon: best.Longitude, Elev: best.Elevation,
		Resolved: true,
	}
	cache.put(key, loc)
	return loc, nil
}

func searchPlaces(ctx context.Context, name string) ([]geocodeResult, error) {
	u := geocodeEndpoint + "?" + url.Values{
		"name":     {name},
		"count":    {"10"},
		"language": {"ja"},
		"format":   {"json"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("地点の検索に失敗: %w", err)
	}
	defer resp.Body.Close()

	var out geocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("地点検索の応答を解析できません: %w", err)
	}
	if out.Error {
		return nil, fmt.Errorf("地点検索: %s", out.Reason)
	}
	return out.Results, nil
}

func geoCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tenki", "geocode.json"), nil
}

func loadGeoCache() *geoCache {
	c := &geoCache{data: map[string]location{}}
	path, err := geoCachePath()
	if err != nil {
		return c
	}
	c.path = path
	b, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, &c.data) // 壊れていたら空のまま使う
	return c
}

func (c *geoCache) get(key string) (location, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	loc, ok := c.data[key]
	return loc, ok
}

func (c *geoCache) put(key string, loc location) {
	if c.path == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = loc
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	b, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", c.path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, c.path)
}
