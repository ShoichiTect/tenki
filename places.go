package main

// Open-Meteo の geocoding は日本語表記の解決が弱い。
//   - 「大阪」「京都」「札幌」はヒットしない（英語名でないと引けない）
//   - 「横浜」は青森県の横浜、「神戸」は三重県の神戸が返る
// そのため (1) 主要地名の漢字→ローマ字エイリアスを持ち、
// (2) feature_code と人口でスコアリングして代表地点を選ぶ。

import (
	"math"
	"strings"
)

// 主要地名のエイリアス（都道府県庁所在地 + 主要都市 + 東京の主要街）。
var placeAlias = map[string]string{
	// 北海道・東北
	"北海道": "Hokkaido", "札幌": "Sapporo", "函館": "Hakodate", "旭川": "Asahikawa",
	"青森": "Aomori", "盛岡": "Morioka", "仙台": "Sendai", "秋田": "Akita",
	"山形": "Yamagata", "福島": "Fukushima", "郡山": "Koriyama", "いわき": "Iwaki",
	// 関東
	"水戸": "Mito", "宇都宮": "Utsunomiya", "前橋": "Maebashi", "高崎": "Takasaki",
	"さいたま": "Saitama", "浦和": "Urawa", "川越": "Kawagoe", "所沢": "Tokorozawa",
	"千葉": "Chiba", "船橋": "Funabashi", "柏": "Kashiwa", "成田": "Narita",
	"東京": "Tokyo", "千代田": "Chiyoda", "中央区": "Chuo", "港区": "Minato",
	"新宿": "Shinjuku", "渋谷": "Shibuya", "池袋": "Ikebukuro", "豊島": "Toshima",
	"品川": "Shinagawa", "上野": "Ueno", "秋葉原": "Akihabara", "六本木": "Roppongi",
	"銀座": "Ginza", "浅草": "Asakusa", "恵比寿": "Ebisu", "中野": "Nakano",
	"吉祥寺": "Kichijoji", "立川": "Tachikawa", "八王子": "Hachioji", "青梅": "Ome",
	"高田馬場": "Takadanobaba", "目黒": "Meguro", "世田谷": "Setagaya", "練馬": "Nerima",
	"羽田": "Haneda", "筑波": "Tsukuba",
	"横浜": "Yokohama", "川崎": "Kawasaki", "鎌倉": "Kamakura", "藤沢": "Fujisawa",
	// 中部
	"新潟": "Niigata", "富山": "Toyama", "金沢": "Kanazawa", "福井": "Fukui",
	"甲府": "Kofu", "長野": "Nagano", "松本": "Matsumoto", "岐阜": "Gifu",
	"静岡": "Shizuoka", "浜松": "Hamamatsu", "名古屋": "Nagoya", "豊田": "Toyota",
	"津": "Tsu", "四日市": "Yokkaichi",
	// 近畿
	"大津": "Otsu", "京都": "Kyoto", "大阪": "Osaka", "堺": "Sakai", "豊中": "Toyonaka",
	"神戸": "Kobe", "姫路": "Himeji", "奈良": "Nara", "和歌山": "Wakayama",
	// 中国・四国
	"鳥取": "Tottori", "松江": "Matsue", "岡山": "Okayama", "倉敷": "Kurashiki",
	"広島": "Hiroshima", "福山": "Fukuyama", "山口": "Yamaguchi", "下関": "Shimonoseki",
	"徳島": "Tokushima", "高松": "Takamatsu", "松山": "Matsuyama", "高知": "Kochi",
	// 九州・沖縄
	"福岡": "Fukuoka", "北九州": "Kitakyushu", "久留米": "Kurume", "佐賀": "Saga",
	"長崎": "Nagasaki", "熊本": "Kumamoto", "大分": "Oita", "宮崎": "Miyazaki",
	"鹿児島": "Kagoshima", "那覇": "Naha", "沖縄": "Okinawa", "石垣": "Ishigaki",
}

// 地名の末尾に付きがちな行政接尾辞。落として再検索する。
var placeSuffixes = []string{"都", "府", "県", "市", "区", "町", "村", "駅"}

// candidateQueries は「元の表記」「接尾辞を落とした表記」「ローマ字」を重複なく返す。
func candidateQueries(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	add(q)

	base := q
	changed := true
	for changed {
		changed = false
		for _, suf := range placeSuffixes {
			// 「港区」「中央区」のように接尾辞込みで意味を持つ語は壊さない
			if len(base) > len(suf) && strings.HasSuffix(base, suf) {
				if _, keep := placeAlias[base]; keep {
					break
				}
				base = strings.TrimSuffix(base, suf)
				changed = true
			}
		}
	}
	if base != q {
		add(base)
	}

	if romaji, ok := placeAlias[base]; ok {
		add(romaji)
	}
	if romaji, ok := placeAlias[q]; ok {
		add(romaji)
	}
	return out
}

// featureRank は地名種別の重要度。大きいほど代表地点らしい。
func featureRank(code string) float64 {
	switch code {
	case "PPLC":
		return 100 // 首都
	case "PPLA":
		return 90 // 県庁所在地・州都
	case "PPLA2":
		return 80
	case "PPLA3":
		return 70
	case "PPLA4":
		return 60
	case "PPL":
		return 50 // 一般集落
	case "PPLX":
		return 40 // 市街地区分
	case "PPLL":
		return 30 // 集落
	}
	return 10
}

// scoreResult は候補の「代表地点らしさ」を測る。大きいほど採用しやすい。
func scoreResult(r geocodeResult, query string) float64 {
	score := featureRank(r.FeatureCode)

	// 表記がそのまま一致するものを強く優遇する（「横浜市」vs「横浜」など）
	name := strings.TrimSpace(r.Name)
	q := strings.TrimSpace(query)
	if name == q {
		score += 30
	} else {
		bare := q
		for _, suf := range placeSuffixes {
			bare = strings.TrimSuffix(bare, suf)
		}
		if name == bare || strings.TrimSuffix(name, "市") == bare {
			score += 20
		}
	}

	// 人口は対数で効かせる（東京 3700万 vs 集落 400 の差を潰しすぎない）
	if r.Population > 0 {
		score += math.Log10(r.Population) * 3
	}
	return score
}
