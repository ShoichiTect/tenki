package main

// WMO 天気コード → 日本語/英語ラベル。
// https://open-meteo.com/en/docs (WMO Weather interpretation codes)

type wmoInfo struct {
	JA string
	EN string
	// Icon は端末で潰れにくい1文字表現。
	Icon string
}

var wmoTable = map[int]wmoInfo{
	0:  {"快晴", "Clear sky", "☀"},
	1:  {"晴れ", "Mainly clear", "☀"},
	2:  {"晴れ時々曇り", "Partly cloudy", "⛅"},
	3:  {"曇り", "Overcast", "☁"},
	45: {"霧", "Fog", "🌫"},
	48: {"霧氷", "Depositing rime fog", "🌫"},
	51: {"弱い霧雨", "Light drizzle", "🌦"},
	53: {"霧雨", "Moderate drizzle", "🌦"},
	55: {"強い霧雨", "Dense drizzle", "🌧"},
	56: {"着氷性の弱い霧雨", "Light freezing drizzle", "🌧"},
	57: {"着氷性の霧雨", "Dense freezing drizzle", "🌧"},
	61: {"弱い雨", "Slight rain", "🌦"},
	63: {"雨", "Moderate rain", "🌧"},
	65: {"強い雨", "Heavy rain", "🌧"},
	66: {"着氷性の弱い雨", "Light freezing rain", "🌧"},
	67: {"着氷性の強い雨", "Heavy freezing rain", "🌧"},
	71: {"弱い雪", "Slight snowfall", "🌨"},
	73: {"雪", "Moderate snowfall", "🌨"},
	75: {"強い雪", "Heavy snowfall", "❄"},
	77: {"霧雪", "Snow grains", "🌨"},
	80: {"弱いにわか雨", "Slight rain showers", "🌦"},
	81: {"にわか雨", "Moderate rain showers", "🌧"},
	82: {"激しいにわか雨", "Violent rain showers", "⛈"},
	85: {"弱いにわか雪", "Slight snow showers", "🌨"},
	86: {"強いにわか雪", "Heavy snow showers", "❄"},
	95: {"雷雨", "Thunderstorm", "⛈"},
	96: {"雷雨と弱い雹", "Thunderstorm with slight hail", "⛈"},
	99: {"雷雨と強い雹", "Thunderstorm with heavy hail", "⛈"},
}

func wmoLookup(code int, lang string) wmoInfo {
	info, ok := wmoTable[code]
	if !ok {
		info = wmoInfo{JA: "不明", EN: "Unknown", Icon: "?"}
	}
	if lang == "en" {
		return info
	}
	return info
}

func wmoLabel(code int) string { return wmoLookup(code, "ja").JA }
func wmoIcon(code int) string  { return wmoLookup(code, "ja").Icon }
func isWetCode(code int) bool  { return code >= 51 && code <= 99 }
