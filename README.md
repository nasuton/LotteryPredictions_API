# LotteryPredictions_API
MySQLに登録されているデータを取得するAPI

## 予測データの一覧取得

`GET /api/predictions` は、日付を制限せず、予測日・IDの降順でデータを返します。
`BASE_PATH` を設定している場合は、パスの先頭に付けてください。

| パラメーター | 動作 |
| --- | --- |
| `lottery_type` | 任意。指定した種別で絞り込みます。 |
| `limit` | 1回の取得件数。省略時は20件、最大100件。100を超える指定は100件に調整します。不正な値や0以下は20件になります。 |
| `offset` | 読み飛ばす件数。省略時、不正な値、負数は0になります。 |

レスポンスには `data`、対象の総件数 `total`、適用した `limit`・`offset`、次のページの `next_url` を返します。
`next_url` は API のオリジンを基準にした相対URLで、種別などのクエリと `BASE_PATH` を引き継ぎます。次のページがなければ `null` です。

例えば112件ある場合、`GET /api/predictions?limit=112` のレスポンスは `data` に100件、`total` に112、`limit` に100、`offset` に0を返し、`next_url` は `/api/predictions?limit=100&offset=100` になります。
そのURLへのリクエストで残り12件を取得でき、`next_url` は `null` になります。

全件取得するクライアントは `next_url` が `null` になるまで取得を繰り返してください。
フロントエンドと API のオリジンが異なる場合は、API のURLを基準に解決します（例：`new URL(next_url, apiBaseUrl)`）。
