# tenki

コマンドライン天気。**APIキー不要**（[Open-Meteo](https://open-meteo.com/)）、依存ゼロ（Go 標準ライブラリのみ）。

```console
$ tenki
高田馬場 東京都 日本  35.7131, 139.7001  標高23m
2026-10-27 (火)  今日 06:00-21:00

 時刻    天気               気温  降水確率            降水量  体感
 06:00   曇り             16.2°C   53% █████░░░░░      0.0mm   17.1°C
 07:00   曇り             16.0°C   47% █████░░░░░      0.0mm   17.1°C
 08:00   曇り             16.2°C   35% ████░░░░░░      0.0mm   17.2°C
 ...
 15:00   晴れ             20.3°C    2% ░░░░░░░░░░      0.0mm   22.1°C
 16:00   晴れ時々曇り     20.1°C    0% ░░░░░░░░░░      0.0mm   21.9°C
 ...

 気温  20.3°C / 16.0°C   最大降水確率  53%   降水量計  0.0mm
```

## インストール

```sh
cd tenki
go install .        # $GOBIN か ~/go/bin に入る
```

## 使い方

```
tenki [フラグ] [地点]
```

地点を省略すると `Tokyo`。日本語・英語どちらでも可（`高田馬場`, `大阪`, `Yokohama`）。`TENKI_PLACE` 環境変数で既定地点を変えられる。

### モード（同時に1つだけ）

| 指定 | 内容 |
|---|---|
| なし | 今日の 06:00〜21:00 を1時間ごと |
| `-now` | 現在の実況（気温・体感・湿度・風・降水） |
| `-hour` | 1時間ごと（既定24時間、`-days N` で延長） |
| `-minute` | 15分ごと（既定3時間先まで、`-hours N` で変更） |
| `-day` | デイリー（`-days N`） |
| `-week` | 7日間のデイリー |

### フラグ

| フラグ | 意味 |
|---|---|
| `-place NAME` | 地点名 |
| `-lat F -lon F` | 座標を直接指定（ジオコーディングを飛ばす） |
| `-days N` | `-hour` / `-day` の日数（1-16） |
| `-hours N` | `-minute` で何時間先まで（1-24） |
| `-json` | JSON で出力 |
| `-unit c\|f` | 気温の単位 |
| `-color auto\|always\|never` | 色付け（`NO_COLOR` / `TERM=dumb` も尊重） |

```sh
tenki -now 新宿
tenki -hour -days 2 大阪
tenki -week 札幌
tenki -minute -hours 1 京都
tenki -now -lat 35.7131 -lon 139.7001
tenki -json | jq '.hourly[] | select(.precipitation_probability >= 50)'
```

## 出力例

`-now`:

```console
新宿 東京都 日本  35.6911, 139.7085  標高44m

  14:45  昼  10/27 (火)

   天気  ☀ 晴れ
   気温  20.1°C
   体感  22.0°C
   湿度  77%
   風    南東 0.4m/s
```

`-week`:

```console
 日付          天気            最高/最低  降水確率            降水量  日の出/日の入
 10/27 (火)    にわか雨      12.4°C/4.8°C  100% ██████████      4.8mm  06:02 / 16:33
 10/28 (水)    弱いにわか雪    5.8°C/2.8°C  100% ██████████      6.5mm  06:03 / 16:32
```

`-json`（抜粋）:

```json
{
  "location": {
    "name": "高田馬場", "admin": "東京都",
    "latitude": 35.7131, "longitude": 139.7001,
    "elevation": 23, "timezone": "Asia/Tokyo"
  },
  "mode": "default",
  "temperature_unit": "celsius",
  "hourly": [
    { "time": "2026-10-27T06:00", "temperature_2m": 16.2,
      "precipitation_probability": 53, "weather_code": 3, "weather": "曇り" }
  ]
}
```

## 解像度について

Open-Meteo の格子は約1〜5km。緯度経度は町丁目単位で指定できるが、**隣接する街の値はほぼ同じになる**（格子セルにスナップし、そのセルの標高で補正しているため）。「高田馬場と新宿で違う値」は期待しないこと。1km の実解像度が必要なのは降水だけで、それは気象庁の降水ナウキャスト（1時間先まで）になる。

## 地名解決の仕組み

Open-Meteo の geocoding は日本語が弱い（`大阪`/`京都` が引けず、`横浜` は青森県の横浜、`神戸` は三重県の神戸が返る）。そこで:

1. 入力・行政接尾辞を落とした形・ローマ字エイリアス（`places.go`）の3通りで検索
2. `feature_code`（PPLC > PPLA > PPL…）と人口でスコアリングして代表地点を選ぶ

結果は `~/Library/Caches/tenki/geocode.json`（`$XDG_CACHE_HOME` 相当）にキャッシュされる。

## 構成

| ファイル | 役割 |
|---|---|
| `main.go` | 引数解析・モード決定・地点解決 |
| `openmeteo.go` | API リクエスト組み立てと取得 |
| `geocode.go` | 地名検索とキャッシュ |
| `places.go` | 漢字→ローマ字エイリアスとスコアリング |
| `wmo.go` | WMO 天気コード → 日本語ラベル |
| `render.go` | テキスト / JSON 出力 |

## テスト

```sh
go test ./...
```

`render_test.go` は色付けあり/なしで**表示幅が変わらない**ことを検証している（ANSI コードを桁計算に混ぜると表が崩れるため、pad → color の順を守る必要がある）。
