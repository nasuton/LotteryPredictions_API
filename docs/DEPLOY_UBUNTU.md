# Ubuntu での実行手順

## 0. 前提

- Ubuntu 22.04 / 24.04
- MySQL 8.0（同一ホスト or 別ホスト）
- Go 1.26 以上（サーバー上でビルドする場合のみ必要）

---

## 1. Go のインストール（サーバーでビルドする場合）

Ubuntu の apt 版は古いことが多いため、公式 tarball を推奨します。

```bash
# 例: go1.26.0
cd /tmp
curl -LO https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
source /etc/profile.d/go.sh
go version
```

---

## 2. MySQL の準備

```bash
sudo apt update
sudo apt install -y mysql-server
sudo systemctl enable --now mysql
```

DB とユーザーを作成し、スキーマを流し込みます。

> **注意:** 以下の `lottery` は MySQL のユーザー名の例です。
> すでに別のユーザー（例: `xusTuc`）を作成済みの場合は、
> 本書の `lottery` を**すべてそのユーザー名に読み替えてください**。
> 重要なのは「MySQL 側のユーザー名」と「`.env` の `DB_USER`」が一致していることです。
> （手順 4 で作る OS ユーザーの `lottery` とは別物なので混同しないでください）

```bash
sudo mysql -u root -p
```

```sql
CREATE DATABASE lottery CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
CREATE USER 'lottery'@'localhost' IDENTIFIED BY '<強いパスワード>';
GRANT SELECT, INSERT, UPDATE, DELETE ON lottery.* TO 'lottery'@'localhost';
FLUSH PRIVILEGES;
```

```bash
mysql -u lottery -p lottery < db/schema.sql
```

### タイムゾーン（重要）

本 API は `predicted_at` が「サーバーの当日日付」と一致するレコードのみ返します。
アプリ（Go）と MySQL の TZ がずれると 0 件になるので揃えてください。

```bash
# OS を JST に
sudo timedatectl set-timezone Asia/Tokyo

# MySQL のタイムゾーンテーブル投入（'Asia/Tokyo' を使う場合に必要）
mysql_tzinfo_to_sql /usr/share/zoneinfo | sudo mysql -u root -p mysql
```

`/etc/mysql/mysql.conf.d/mysqld.cnf` に追記:

```ini
[mysqld]
default-time-zone = 'Asia/Tokyo'
```

```bash
sudo systemctl restart mysql
```

---

## 3. ソース取得とビルド

```bash
sudo mkdir -p /opt/lottery-api
sudo chown "$USER":"$USER" /opt/lottery-api

git clone <リポジトリURL> ~/src/LotteryPredictions_API
cd ~/src/LotteryPredictions_API

# 依存取得 & ビルド（静的寄りのバイナリ）
go mod download
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o lottery-api .

cp lottery-api /opt/lottery-api/
```

### Windows からクロスコンパイルして転送する場合

PowerShell:

```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o lottery-api .
scp .\lottery-api user@server:/opt/lottery-api/
# 使い終わったら戻す
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

転送後に実行権限を付与:

```bash
chmod +x /opt/lottery-api/lottery-api
```

---

## 4. 実行用ユーザーの作成

以降の `chown` で使うため、先にサービス実行用のシステムユーザーを作成します。
（これを飛ばすと `chown: invalid user: 'lottery:lottery'` になります）

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin lottery
```

すでに存在するか確認したい場合:

```bash
id lottery
```

> `useradd` は同名グループ `lottery` も同時に作成します。
> もし `--no-user-group` などでグループが作られていない場合は
> `sudo groupadd --system lottery && sudo usermod -g lottery lottery` を実行してください。

---

## 5. .env の扱い

### ポート番号について

アプリの待ち受けポートは `.env` の `PORT` で決まります。
**未設定の場合のデフォルトは `8080`** です（`internal/config/config.go`）。

すでに 8080 を他のアプリが使っている場合は必ず変更してください。

使用中のポートを確認:

```bash
# 8080 が使われているか
sudo ss -lptn 'sport = :8080'

# LISTEN 中のポート一覧
sudo ss -lptn
```

空いているポート（例: `8090`）を選んで `.env` に設定します。

```dotenv
PORT=8090
```

変更した場合、以下も合わせて修正が必要です。

- Nginx の `proxy_pass http://127.0.0.1:8090;`（手順 8）
- 動作確認の `curl http://127.0.0.1:8090/health`
- ufw で直接公開している場合のポート開放

> 1024 未満のポート（80/443 など）は root 権限が必要です。
> `lottery` ユーザーで直接バインドはできないため、
> Nginx でリバースプロキシする構成を推奨します。
> どうしても直接使う場合は次の Capability を付与します。
>
> ```bash
> sudo setcap 'cap_net_bind_service=+ep' /opt/lottery-api/lottery-api
> ```

---

このアプリの `config.Load()` は次の順で動きます。

1. 環境変数 `ENV_FILE` があればそのパス、なければ **カレントディレクトリの `.env`**
2. ファイルが無くてもエラーにはならず、**OS の環境変数**だけで動く
3. どちらにも無いキャストはコード内のデフォルト値（`DB_USER=root` など）

つまり `.env` は必須ではありませんが、設定を一箇所にまとめられるので推奨です。

### 5-1. .env を使う方法（おすすめ・シンプル）

```bash
cd /opt/lottery-api
cp ~/src/LotteryPredictions_API/.env.example .env
nano .env
```

本番用の例:

```dotenv
DB_USER=lottery
DB_PASSWORD=<強いパスワード>
DB_HOST=127.0.0.1
DB_PORT=3306
DB_NAME=lottery

DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=25
DB_CONN_MAX_LIFETIME=5m

PORT=8080
GIN_MODE=release

# Nginx を同一ホストでリバースプロキシする場合
TRUSTED_PROXIES=127.0.0.1

ALLOWED_ORIGINS=https://<ユーザー名>.github.io
CORS_MAX_AGE=12h
```

パーミッションを絞ります（パスワードを含むため）。

```bash
sudo chown lottery:lottery /opt/lottery-api/.env
sudo chmod 600 /opt/lottery-api/.env
```

> 注意: `.env` は **カレントディレクトリ基準** で読まれます。
> systemd では `WorkingDirectory=/opt/lottery-api` を必ず指定してください。
> 別の場所に置きたい場合は `Environment=ENV_FILE=/etc/lottery-api/.env` を使います。

`.env` は `.gitignore` 済みなので Git にはコミットされません。

### 5-2. systemd の EnvironmentFile を使う方法

`.env` ファイルをアプリに読ませず、systemd から環境変数として渡す方法です。

```bash
sudo mkdir -p /etc/lottery-api
sudo cp .env.example /etc/lottery-api/lottery-api.env
sudo nano /etc/lottery-api/lottery-api.env
sudo chown root:lottery /etc/lottery-api/lottery-api.env
sudo chmod 640 /etc/lottery-api/lottery-api.env
```

ユニットファイルを配置し、`EnvironmentFile` 行のコメント（先頭の `#`）を外します。

```bash
# まだ配置していなければコピー
sudo cp deploy/lottery-api.service /etc/systemd/system/

# 編集
sudo nano /etc/systemd/system/lottery-api.service
```

`deploy/lottery-api.service` の該当箇所は最初こうなっています。

```ini
# .env の代わりに systemd 側で環境変数を渡したい場合は以下を使う
# (この場合 WorkingDirectory に .env は不要)
# EnvironmentFile=/etc/lottery-api/lottery-api.env
```

3 行目の先頭 `#` を削除して、次の状態にします。

```ini
EnvironmentFile=/etc/lottery-api/lottery-api.env
```

`sed` で一括置換しても構いません。

```bash
sudo sed -i 's|^# *EnvironmentFile=|EnvironmentFile=|' /etc/systemd/system/lottery-api.service
```

編集後は必ず再読み込みして再起動します。

```bash
sudo systemctl daemon-reload
sudo systemctl restart lottery-api
sudo systemctl status lottery-api
```

反映されたか確認するには次のコマンドを使います。

```bash
# ユニットに設定された EnvironmentFile のパスを確認
systemctl show lottery-api -p EnvironmentFiles
```

`EnvironmentFiles=/etc/lottery-api/lottery-api.env (ignore_errors=no)` のように
パスが表示されれば有効化できています。

> **注意:** `systemctl show lottery-api -p Environment` は **空欄になります**。
> このプロパティはユニット内の `Environment=` で直接書いた変数だけを表示し、
> `EnvironmentFile=` の中身は起動時に展開されるため含まれません。
> 空欄でも異常ではありません。

実際にプロセスへ渡っている環境変数を見たい場合は `/proc` を確認します。

```bash
PID=$(systemctl show lottery-api -p MainPID --value)
sudo tr '\0' '\n' < /proc/$PID/environ | sort
```

`DB_USER=lottery` などが並んでいれば正しく渡っています。

アプリ側から見た確認としては、起動ログで DB 接続に成功しているかが実質的な判断材料です。

```bash
journalctl -u lottery-api -n 30 --no-pager
curl http://127.0.0.1:8080/health
```

> `EnvironmentFiles` 自体が空の場合は、
> 編集したファイルが `/etc/systemd/system/lottery-api.service` か、
> `#` が確実に外れているか、`sudo systemctl daemon-reload` を実行したかを確認してください。
>
> ```bash
> grep -n EnvironmentFile /etc/systemd/system/lottery-api.service
> systemctl cat lottery-api
> ```

> systemd の `EnvironmentFile` は `KEY=VALUE` と `#` コメントのみ対応です。
> `export` 記法や複雑なクォートは使わないでください。

> この方式でも `WorkingDirectory` の `.env` が存在すればそちらが優先的に読み込まれます。
> 設定が二重にならないよう、`/opt/lottery-api/.env` は削除しておいてください。
>
> ```bash
> sudo rm -f /opt/lottery-api/.env
> ```
>
> `.env` が無い場合、ログに「.env が見つからないため環境変数のみを使用します」と出ますが正常です。

---

## 6. 動作確認（フォアグラウンド）

`.env` は `lottery` ユーザー所有の 600 なので、`lottery` ユーザーとして実行します。

```bash
cd /opt/lottery-api
sudo -u lottery ./lottery-api
```

別ターミナルから:

```bash
curl http://127.0.0.1:8080/health
curl "http://127.0.0.1:8080/api/predictions?lottery_type=loto6&limit=10"
```

---

## 7. systemd サービス化

ディレクトリの所有者を実行ユーザーに合わせます。

```bash
sudo chown -R lottery:lottery /opt/lottery-api
```

ユニットを配置して起動します。

```bash
sudo cp deploy/lottery-api.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now lottery-api
sudo systemctl status lottery-api
```

ログ確認:

```bash
journalctl -u lottery-api -f
```

更新時（バイナリ差し替え）:

```bash
sudo systemctl stop lottery-api
sudo cp lottery-api /opt/lottery-api/
sudo chown lottery:lottery /opt/lottery-api/lottery-api
sudo systemctl start lottery-api
```

---

## 8. Nginx リバースプロキシ + HTTPS（任意）

```bash
sudo apt install -y nginx
```

### 8-1. サブパス `/lottery/api` で公開する場合（推奨）

`https://nasuton.com/lottery/api/predictions` のように、
既存サイトのサブパスとして公開する構成です。

方法は 2 つあり、**どちらか一方**を選びます。

#### 方法 A: Nginx でプレフィックスを削る（アプリは `BASE_PATH` 未設定のまま）

`proxy_pass` の末尾にスラッシュを付けると `/lottery/` の部分が削られて転送されます。

```nginx
server {
    listen 80;
    server_name nasuton.com;

    # 末尾の "/" が重要。/lottery/api/... -> /api/... に変換されて転送される
    location /lottery/ {
        proxy_pass         http://127.0.0.1:8080/;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_set_header   X-Forwarded-Prefix /lottery;
    }
}
```

`.env`:

```dotenv
BASE_PATH=
```

#### 方法 B: アプリ側にプレフィックスを持たせる（`BASE_PATH=/lottery`）

`proxy_pass` の末尾にスラッシュを付けず、パスをそのまま転送します。

```nginx
server {
    listen 80;
    server_name nasuton.com;

    # 末尾に "/" を付けない。/lottery/api/... がそのまま転送される
    location /lottery/ {
        proxy_pass         http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

`.env`:

```dotenv
BASE_PATH=/lottery
```

この場合、アプリのエンドポイントは次のようになります。

| URL | 内容 |
|---|---|
| `https://nasuton.com/lottery/health` | ヘルスチェック |
| `https://nasuton.com/lottery/api/predictions` | 一覧 |
| `https://nasuton.com/lottery/api/predictions/1` | 個別取得 |

> **方法 A と B を同時に設定しないでください。**
> `proxy_pass` 末尾スラッシュあり + `BASE_PATH=/lottery` にすると
> パスが二重に外れて 404 になります。
> どちらを使っているか迷ったら、起動ログの `config: BASE_PATH=...` を確認してください。

### 8-2. 専用ドメインのルート直下で公開する場合

`https://api.example.com/api/predictions` のような構成です。

```nginx
server {
    listen 80;
    server_name api.example.com;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

`.env` は `BASE_PATH=`（空）のままにします。

### 8-3. 有効化と HTTPS

```bash
sudo ln -s /etc/nginx/sites-available/lottery-api /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx

# HTTPS
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d nasuton.com
```

Nginx を挟む場合は `.env` に `TRUSTED_PROXIES=127.0.0.1` を設定してください。
また CORS を使うフロントからアクセスする場合は `ALLOWED_ORIGINS` に
`https://nasuton.com` を含めます。

動作確認:

```bash
curl -i https://nasuton.com/lottery/health
curl -i "https://nasuton.com/lottery/api/predictions?lottery_type=loto6&limit=5"
```

### ファイアウォール

```bash
sudo ufw allow OpenSSH
sudo ufw allow 'Nginx Full'
sudo ufw enable
```

Nginx を使わずアプリを直接公開する場合のみ `sudo ufw allow 8080/tcp` を追加します。

---

## 9. トラブルシューティング

### `Access denied for user 'root'@'localhost'` が出る

```
config: .env が見つからないため環境変数のみを使用します
database connection error: failed to ping mysql: Error 1045 (28000): Access denied for user 'root'@'localhost'
```

`root` が出ている時点で **設定が一切読めておらず、コード内のデフォルト値
（`DB_USER=root` / `DB_PASSWORD=password`）で接続を試みています**。
MySQL の権限ではなく、環境変数の受け渡しが原因です。

原因は次の 3 パターンのいずれかです。

**A) カレントディレクトリに `.env` が無い（手動実行時）**

`.env` は実行時のカレントディレクトリ基準で探されます。

```bash
# NG: どこから実行したかで結果が変わる
/opt/lottery-api/lottery-api

# OK: ディレクトリを移動してから実行
cd /opt/lottery-api && sudo -u lottery ./lottery-api

# または絶対パスを明示
sudo -u lottery ENV_FILE=/opt/lottery-api/.env /opt/lottery-api/lottery-api
```

ファイルの有無と読み取り権限を確認します。

```bash
ls -l /opt/lottery-api/.env
sudo -u lottery cat /opt/lottery-api/.env   # 読めなければ権限の問題
```

`.env` を削除して 5-2 の EnvironmentFile 方式に切り替えた場合は、
手動実行では環境変数が渡らないため必ずこのエラーになります。
その場合は `systemctl start` 経由で起動してください。

**B) EnvironmentFile が効いていない（systemd 実行時）**

```bash
systemctl show lottery-api -p EnvironmentFiles   # 空なら未反映
grep -n EnvironmentFile /etc/systemd/system/lottery-api.service
sudo systemctl daemon-reload && sudo systemctl restart lottery-api
```

**C) env ファイルの中身が未編集**

`.env.example` をコピーしたまま値を書き換えていないケースです。

```bash
grep -E '^DB_(USER|PASSWORD|HOST|NAME)=' /opt/lottery-api/.env
# または
sudo grep -E '^DB_(USER|PASSWORD|HOST|NAME)=' /etc/lottery-api/lottery-api.env
```

`DB_USER=lottery` と手順 2 で設定したパスワードになっているか確認します。
値にスペースや `#`、日本語が含まれる場合はクォートせず、記号の少ないパスワードにしてください。

**確認: MySQL 側の資格情報が正しいか**

```bash
mysql -u lottery -p -h 127.0.0.1 lottery -e "SELECT CURDATE();"
```

ここで失敗するなら MySQL 側の問題です。パスワードを再設定します。

```sql
ALTER USER 'lottery'@'localhost' IDENTIFIED BY '<新しいパスワード>';
FLUSH PRIVILEGES;
```

> `DB_HOST=127.0.0.1` の場合 MySQL からは TCP 接続として扱われます。
> ユーザーを `'lottery'@'localhost'` で作っていれば通常は同一視されますが、
> 念のため `CREATE USER 'lottery'@'127.0.0.1'` も作っておくと確実です。

**起動ログでの確認**

起動時に接続先の概要が出力されます（パスワードは伏字）。

```bash
journalctl -u lottery-api -n 30 --no-pager
```

```
config: DB接続先 = lottery:****@tcp(127.0.0.1:3306)/lottery
```

ここが `root:****@...` になっていたら設定が読めていません。

---

### その他

| 症状 | 確認ポイント |
|---|---|
| `chown: invalid user: 'lottery:lottery'` | `lottery` ユーザー未作成。手順 4 の `useradd` を先に実行する（`id lottery` で確認） |
| `systemctl show -p Environment` が空 | 正常。`EnvironmentFile` の中身は表示されない。`-p EnvironmentFiles` か `/proc/<PID>/environ` で確認する |
| `database connection error` | `.env` の DB_USER / DB_PASSWORD / DB_HOST、`mysql -u lottery -p` で手動接続できるか |
| `.env` が読まれない | `WorkingDirectory` が `/opt/lottery-api` になっているか、`journalctl` に「.env を読み込みました」が出ているか |
| API が常に空配列を返す | `predicted_at` が当日日付のレコードが存在するか。`SELECT CURDATE(); SELECT DISTINCT predicted_at FROM lottery_predictions;` |
| 日付が 1 日ずれる | OS と MySQL の TZ（手順 2 のタイムゾーン設定） |
| ブラウザで CORS エラー | `ALLOWED_ORIGINS` にフロントのオリジンを完全一致で指定（末尾スラッシュ不要） |
| `Permission denied` | `chmod +x /opt/lottery-api/lottery-api`、所有者が `lottery` か |
| `bind: address already in use` / ポート 8080 が使用中 | `sudo ss -lptn 'sport = :8080'` で使用中プロセスを確認し、`.env` の `PORT` を空きポート（例 8090）に変更して再起動 |

