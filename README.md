# LotteryPredictions_API

MySQL に登録された宝くじ予測データ (`lottery_predictions`) を返す読み取り専用 API です。
Go 1.26 / Gin 製。バッチ側 ([nasuton/Lottery](https://github.com/nasuton/Lottery)) が書き込み、
フロント ([nasuton/LotteryPredictions](https://github.com/nasuton/LotteryPredictions), GitHub Pages) が参照します。

本番は Linux VPS 上で systemd 管理、Apache のリバースプロキシ (`https://nasuton.com/lottery`) の後ろで動かす想定です。

## エンドポイント

すべて `GET`。`BASE_PATH` (例: `/lottery`) を設定している場合はパスの先頭に付きます。

| パス | 説明 |
| --- | --- |
| `/api/v1/predictions` | 予測データ一覧 (ページング) |
| `/api/v1/predictions/:id` | 予測データ 1 件 |
| `/api/v1/status` | 種別ごとの件数・最新予測日と、バッチの最終実行状況 |
| `/healthz` | liveness。DB に触れず常に `200 {"status":"ok"}` |
| `/readyz` | readiness。DB に `PingContext` (1 秒) が通れば `200 {"status":"ok"}`、失敗なら `503 {"status":"ng"}` |
| `/api/predictions`, `/api/predictions/:id` | **互換用 (無印)**。v1 と同じ handler。**将来廃止予定**なので新規クライアントは v1 を使ってください |
| `/health` | **互換用**。`/readyz` と同じ挙動 |

無印パスと `/health` のレスポンス形 (`data`, `total`, `limit`, `offset`, `next_url`, `{"status":"ok"}`) は変更していません。

### 一覧取得 `GET /api/v1/predictions`

日付を制限せず、`predicted_at` 降順・`id` 降順で返します。

| パラメーター | 動作 |
| --- | --- |
| `lottery_type` | 任意。`loto6` / `loto7` / `miniloto` / `numbers3` / `numbers4` のいずれか。それ以外は `400 invalid_lottery_type` |
| `limit` | 1 回の取得件数。省略時 20、最大 100。100 超は 100 に、不正値や 0 以下は 20 に調整 |
| `offset` | 読み飛ばす件数。省略時・不正値・負数は 0 |

```json
{
  "data": [
    {"id": 305, "lottery_type": "loto6", "pattern": "...", "predicted_at": "2026-09-28",
     "numbers": ["1","5","12","23","34","41"], "numbers_raw": "1,5,12,23,34,41"}
  ],
  "total": 305,
  "limit": 20,
  "offset": 0,
  "next_url": "/lottery/api/v1/predictions?limit=20&offset=20"
}
```

`next_url` は API のオリジンを基準にした相対 URL で、クエリ・`BASE_PATH`・**呼ばれたパス** (v1 なら v1、無印なら無印) を引き継ぎます。
次のページが無ければ `null` です。全件取得するクライアントは `next_url` が `null` になるまで繰り返してください
(オリジンが異なる場合は `new URL(next_url, apiBaseUrl)` で解決)。

例: 112 件ある場合、`GET /api/v1/predictions?limit=112` は `data` 100 件、`total` 112、`limit` 100、`offset` 0、
`next_url` は `/api/v1/predictions?limit=100&offset=100` になります。

### 単一取得 `GET /api/v1/predictions/:id`

`{"data": {...}}` を返します。`id` が正の整数でなければ `400 invalid_id`、存在しなければ `404 not_found`。

### 状況取得 `GET /api/v1/status`

```json
{
  "data": {
    "predictions": {
      "total": 305,
      "by_type": [
        {"lottery_type": "loto6", "count": 43, "latest_predicted_at": "2026-09-28"}
      ]
    },
    "last_batch_runs": [
      {"batch_name": "registration", "lottery_type": "loto6", "status": "success",
       "started_at": "2026-09-28T03:00:00+09:00", "finished_at": "2026-09-28T03:05:12+09:00",
       "rows_affected": 43, "message": ""}
    ],
    "generated_at": "2026-09-28T12:00:00+09:00"
  }
}
```

- `by_type` は `lottery_type` ごとの `COUNT(*)` と `MAX(predicted_at)`。
- `last_batch_runs` は `batch_runs` テーブルから `batch_name` × `lottery_type` ごとの最新 1 件。
  **`batch_runs` テーブルが存在しない (MySQL error 1146) 場合は `null` を返して 200** のまま動き、ログに WARNING を出します
  (バッチ側のスキーマ移行前でも API は落ちません)。テーブルはあるが空なら `[]`。

## レスポンスヘッダ

| ヘッダ | 対象 | 値 |
| --- | --- | --- |
| `Cache-Control` | 一覧・単一取得 | `public, max-age=300, stale-while-revalidate=600` |
| `Cache-Control` | `/api/v1/status` | `public, max-age=300` |
| `Cache-Control` | エラー・ヘルスチェック | `no-store` |
| `ETag` | 一覧・単一取得 | 弱い ETag (`W/"<sha1>"`)。一覧は `lottery_type|limit|offset|MAX(predicted_at)|COUNT(*)|MAX(id)`、単一はレコード内容から算出。`updated_at` 列に依存しないため列が無い DB でも動く |
| `X-Request-ID` | 全レスポンス | リクエストに付いていれば流用 (印字可能 ASCII・128 文字以内)、無ければ 16 byte の乱数 hex |

`If-None-Match` が `ETag` と (弱い比較で) 一致すれば本文なしの `304 Not Modified` を返し、一覧では DB からの本体取得をスキップします。

## エラーレスポンス

すべてのエラーは次の形に統一されています。

```json
{"error": {"code": "invalid_id", "message": "id must be a positive integer", "request_id": "3f2a…"}}
```

| HTTP | `code` | 発生条件 |
| --- | --- | --- |
| 400 | `invalid_lottery_type` | `lottery_type` が許可値以外 |
| 400 | `invalid_id` | `:id` が正の整数でない |
| 404 | `not_found` | レコードが無い / ルートが無い |
| 500 | `internal_error` | DB エラーや panic。詳細はクライアントに返さずログにのみ出力 |

`request_id` はログの `request_id` と一致するので、問い合わせ時に突き合わせに使えます。

## 環境変数

`.env.example` に全キーがあります。読み込み順は `ENV_FILE` (既定 `./.env`) → OS 環境変数 (既に設定済みの変数は `.env` で上書きしません)。
形式不正 (期間・整数・列挙値) は起動時にまとめてエラーになり exit 1 します。

| キー | 既定 | 説明 |
| --- | --- | --- |
| `DB_PASSWORD` | **必須** | 未設定なら起動失敗 |
| `DB_NAME` | **必須** | 未設定なら起動失敗 |
| `DB_USER` / `DB_HOST` / `DB_PORT` | `root` / `127.0.0.1` / `3306` | |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | `25` / `25` | コネクションプール |
| `DB_CONN_MAX_LIFETIME` | `5m` | |
| `DB_QUERY_TIMEOUT` | `3s` | リクエスト単位の DB 操作タイムアウト (`context.WithTimeout`) |
| `PORT` | `8080` | |
| `GIN_MODE` | `release` | `debug` / `release` / `test` |
| `BASE_PATH` | (なし) | 全ルートの先頭に付けるパス。例 `/lottery` |
| `TRUSTED_PROXIES` | (なし=信頼しない) | `ClientIP()` 判定で信頼するプロキシ (カンマ区切り IP/CIDR) |
| `ALLOWED_ORIGINS` | (なし=**不許可**) | CORS 許可オリジン (カンマ区切り)。例 `https://nasuton.github.io` |
| `CORS_MAX_AGE` | `12h` | プリフライトのキャッシュ時間 |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | `http.Server` の各タイムアウト (Go の期間文字列) |
| `HTTP_READ_TIMEOUT` | `10s` | |
| `HTTP_WRITE_TIMEOUT` | `15s` | |
| `HTTP_IDLE_TIMEOUT` | `60s` | |
| `HTTP_MAX_HEADER_BYTES` | `1048576` | |
| `SHUTDOWN_TIMEOUT` | `10s` | graceful shutdown で処理中リクエストを待つ上限 |
| `LOG_FORMAT` | `json` | `json` / `text` |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `ENV_FILE` | `.env` | 読み込む env ファイル |

## ログ

標準ライブラリ `log/slog` で stdout に出力します (既定 JSON)。リクエストごとに
`method, path, status, latency_ms, client_ip, request_id, user_agent, bytes` を 1 行で記録し、
4xx は `WARN`、5xx は `ERROR` になります。`/healthz` の成功は記録しません。
内部エラーの詳細 (SQL エラー文言など) はログにのみ出し、クライアントには返しません。

## graceful shutdown

`SIGINT` / `SIGTERM` を受けると新規接続の受け付けを止め、処理中のリクエストを最大 `SHUTDOWN_TIMEOUT` 待ってから
DB 接続を閉じて終了します。systemd の `TimeoutStopSec` (15 秒) より短く設定してください。

## ビルド・テスト

```sh
go vet ./...
go test ./...          # DB 不要 (repository は mock)
gofmt -l .             # 何も出なければ OK
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o lottery-api .
```

CI (`.github/workflows/ci.yml`) は push / PR で `gofmt` チェック、`go vet`、`go test -race`、`go build` を実行します。

## デプロイ

### systemd (VPS)

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin lottery-api
sudo install -d /opt/lottery-api
sudo install -m 755 ./lottery-api /opt/lottery-api/lottery-api
sudo install -o root -g lottery-api -m 640 .env /etc/lottery-api.env   # 実値を記入
sudo cp deploy/lottery-api.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now lottery-api
journalctl -u lottery-api -f
```

`deploy/lottery-api.service` は非 root ユーザー、`EnvironmentFile=/etc/lottery-api.env`、`Restart=on-failure`、
`KillSignal=SIGTERM`、`TimeoutStopSec=15`、`ProtectSystem=strict` などの hardening を含みます。

### Apache リバースプロキシ

`deploy/apache-reverse-proxy.conf.example` を既存の HTTPS VirtualHost に追記します
(`a2enmod proxy proxy_http headers` が必要)。API 側は `BASE_PATH=/lottery`, `TRUSTED_PROXIES=127.0.0.1,::1` にします。

```sh
curl -i https://nasuton.com/lottery/healthz
curl -i https://nasuton.com/lottery/readyz
curl -i "https://nasuton.com/lottery/api/v1/predictions?lottery_type=loto6&limit=5"
curl -i https://nasuton.com/lottery/api/v1/status
```

### Docker

```sh
docker build -t lottery-api .
docker run --rm -p 8080:8080 --env-file .env lottery-api
```

multi-stage、`CGO_ENABLED=0`、alpine、非 root、`HEALTHCHECK` は `/healthz` を叩きます。

## 挙動変更一覧 (旧バージョンからの移行)

| 項目 | 旧 | 新 | VPS 側で必要な対応 |
| --- | --- | --- | --- |
| `DB_PASSWORD` / `DB_NAME` | 既定値 `password` / `lottery` | **必須**。未設定なら起動失敗 | `/etc/lottery-api.env` に両方を明記 |
| `ALLOWED_ORIGINS` 未設定 | 起動時に panic (gin-contrib/cors の制約) | CORS ヘッダを一切付けず起動 (不許可) + WARNING | GitHub Pages から使うなら `https://nasuton.github.io` を設定 |
| `GIN_MODE` 既定 | `debug` | `release` | 不要 |
| 環境変数の形式不正 | ログを出して既定値で続行 | 起動失敗 | 値を確認 |
| エラーレスポンス | `{"error": "message"}` | `{"error": {"code", "message", "request_id"}}` | フロントはエラー本文を読んでいないため対応不要 |
| 500 の本文 | 固定文言 | 固定文言 (変更なし)。詳細はログのみ | 不要 |
| ログ形式 | Gin 既定のテキスト | slog JSON (`LOG_FORMAT=text` で切替) | journald / ログ集約の設定を JSON 前提に |
| 停止 | 即時終了 | SIGTERM で graceful shutdown | `deploy/lottery-api.service` に差し替え |
| 新規ヘッダ | なし | `ETag`, `Cache-Control`, `X-Request-ID` | Apache 側で上書きしない |
| 新規パス | なし | `/api/v1/*`, `/healthz`, `/readyz` | Apache の `ProxyPass /lottery` 配下なので追加設定不要 |
| `lottery_type` | 任意文字列 | 5 種別のみ、それ以外は 400 | 不要 |
| `.gitignore` | `.env*` | `.env.example` のみ除外解除 | 不要 |

`nasuton/Lottery` 側で `batch_runs` テーブルと `updated_at` 列を追加する移行 (`db/schema.sql`, `db/migrations/`) を適用していなくても API は動作します。
