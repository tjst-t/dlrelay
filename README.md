# dlrelay

ブラウザで検出した動画をNASにダウンロードするシステム。

拡張機能またはブックマークレットが動画URL（HLS/DASH/直接ストリーム）を自動検出し、NAS上のサーバーでダウンロードを実行する。

## アーキテクチャ

```
クライアント                             NAS
┌─────────────────┐    HTTPS     ┌──────────────────────┐
│  DL Relay 拡張  │              │  Caddy (リバースプロキシ) │
│  ├─ 動画検出      │              │      ↓                │
│  ├─ 画質選択      │ ──────────→ │  dlrelay-server     │
│  └─ DLボタン     │              │  ├─ REST API          │
│                  │              │  ├─ HTTPダウンローダー   │
│  Bookmarklet    │              │  ├─ HLS/DASHダウンローダー│
│  (モバイル対応)   │ ──────────→ │  ├─ yt-dlp            │
│                  │              │  └─ FFmpeg (変換/結合)  │
└─────────────────┘              │      ↓                │
                                 │  /tank/downloads/     │
                                 └──────────────────────┘
```

## コンポーネント

| コンポーネント | 説明 |
|---|---|
| **dlrelay-server** | NAS上のDockerコンテナ。ダウンロード実行・FFmpeg変換・進捗管理 |
| **DL Relay 拡張** | ブラウザ拡張機能。動画を自動検出し、サーバーにダウンロードを指示 |
| **Bookmarklet** | 拡張機能が使えないモバイルブラウザ向けのJavaScriptブックマークレット |

## 必要な外部ツール

サーバー側に以下のツールが必要（Docker イメージには ffmpeg が含まれる）:

| ツール | 用途 |
|---|---|
| **ffmpeg** | HLS/DASHの結合・変換・リムックス |
| **ffprobe** | メディア情報の取得 |
| **yt-dlp** | YouTube等のサイト固有ダウンロード（任意） |
| **Node.js** | yt-dlp の JS ランタイム（YouTube の n-challenge 解決に必要） |
| **curl_cffi** | yt-dlp の HTTP バックエンド（TLS フィンガープリント偽装でブロック回避） |

## クイックスタート

### 1. サーバー起動

```bash
# Docker
docker build -t dlrelay-server .
docker run -d \
  -p 8090:8090 \
  -v /tank/downloads:/downloads \
  -e MAX_CONCURRENT=3 \
  dlrelay-server

# または直接ビルドして実行
make build-server
./bin/dlrelay-server
```

### 2. 拡張機能インストール

サーバーの Extension ページ (`http://your-nas:8090/setup`) にアクセスし、「Extension をダウンロード」ボタンをクリック。サーバーURLが設定済みの拡張がダウンロードされる。

**Chrome / Edge:**
1. zipをダウンロードし展開
2. `chrome://extensions` を開き「デベロッパーモード」ON
3. 「パッケージ化されていない拡張機能を読み込む」で展開フォルダを選択

**Firefox:**
1. zipをダウンロード
2. `about:debugging#/runtime/this-firefox` を開く
3. 「一時的なアドオンを読み込む」でzip内の manifest.json を選択

### 3. ブックマークレット（モバイル向け）

拡張機能が使えない環境では、Bookmarklet ページ (`http://your-nas:8090/bookmarklet`) からブックマークレットを登録できる。

1. ブックマークレットページでコードをコピー
2. ブラウザのブックマークを新規作成し、URL欄にコードを貼り付け
3. 動画のあるページでブックマークを実行

> **注意:** Chromium系ブラウザはペースト時に `javascript:` プレフィックスを削除する。貼り付け後、先頭に `javascript:` を手入力で追加すること。

### 4. 使い方

1. 動画のあるページを開く → 拡張が自動検出しバッジに数を表示
2. 拡張のポップアップを開く → 検出された動画一覧が表示
3. HLS/DASHは画質を選択可能
4. 「Download」ボタンでNASへダウンロード開始
5. ダウンロード状況は `http://your-nas:8090/` で確認

## 拡張機能の検出方式

| 方式 | 説明 |
|---|---|
| **webRequest** | ネットワーク監視で動画MIME/拡張子を検出 |
| **DOM** | `<video>`/`<audio>`タグのsrc属性を収集 |
| **HLS解析** | M3U8マスタープレイリストから画質バリアントを抽出 |
| **DASH解析** | MPDマニフェストから映像+音声の組み合わせを抽出 |
| **blob: フック** | MediaSource APIフックで元URLを逆引き |
| **サイト固有** | Twitter, Instagram, TikTok, YouTube, Reddit, ニコニコのAPI傍受 |

## WebUI

| URL | ページ |
|---|---|
| `/` | ダウンロード一覧・進捗ダッシュボード |
| `/setup` | 拡張機能のセットアップガイド |
| `/bookmarklet` | ブックマークレットの設定ガイド |

## サーバー設定

設定は **TOML設定ファイル** と **環境変数** の2つの方法で行える。環境変数は常にTOMLファイルより優先される。

### TOML設定ファイル

デフォルトパス: `/etc/dlrelay/config.toml`（`CONFIG_FILE` 環境変数で変更可能）

```toml
listen_addr = ":8090"
download_dir = "/downloads"
# temp_dir = "/tmp"
max_concurrent = 3
# extension_dir = "/path/to/extension"
# api_key = "your-secret-key"

# 重複チェック用ディレクトリ
# check_dirs = ["/media/videos", "/media/archive"]

# ドメイン別ダウンロード先
# [[download_rules]]
# domain = "youtube.com"
# dir = "/downloads/youtube"
#
# [[download_rules]]
# domain = "twitter.com"
# dir = "/downloads/twitter"
```

サンプルファイル: [`config.example.toml`](config.example.toml)

### 環境変数

| 変数 | デフォルト | 説明 |
|---|---|---|
| `CONFIG_FILE` | `/etc/dlrelay/config.toml` | TOML設定ファイルのパス |
| `LISTEN_ADDR` | `:8090` | リッスンアドレス |
| `DOWNLOAD_DIR` | `/downloads` | ダウンロード保存先 |
| `TEMP_DIR` | システム依存 | 一時ファイル |
| `MAX_CONCURRENT` | `3` | 最大同時ダウンロード数 |
| `EXTENSION_DIR` | (なし) | 拡張機能ソースディレクトリ（WebUIからのDL用） |
| `API_KEY` | (なし) | API認証キー（設定時、書き込みAPIに `X-API-Key` ヘッダーが必要） |
| `DOWNLOAD_RULES` | (なし) | ドメイン別ダウンロード先ルール（`domain:/path` のカンマ区切り） |
| `CHECK_DIRS` | (なし) | 重複チェック用ディレクトリ（カンマ区切り） |

すべてのパス設定で `~` によるホームディレクトリ展開が使える。

### ドメイン別ダウンロード先 (`DOWNLOAD_RULES`)

ドメインごとに異なるディレクトリにダウンロードできる。

```bash
DOWNLOAD_RULES="youtube.com:/downloads/youtube,twitter.com:/downloads/twitter"
```

- カンマ区切りで複数ルールを指定
- サブドメインは自動マッチ（`youtube.com` は `www.youtube.com` にもマッチ）
- どのルールにもマッチしない場合は `DOWNLOAD_DIR` に保存
- リクエストの `directory` フィールドが指定されていれば、ルールで決まったディレクトリ配下のサブディレクトリとして使用

Docker での使用例:

```bash
docker run -d \
  -p 8090:8090 \
  -v /tank/downloads:/downloads \
  -v /tank/downloads/youtube:/downloads/youtube \
  -e DOWNLOAD_RULES="youtube.com:/downloads/youtube" \
  -e MAX_CONCURRENT=3 \
  dlrelay-server
```

### 重複スキップ (`CHECK_DIRS`)

ダウンロード開始前に既存ファイルをチェックし、同名ファイルが見つかった場合はダウンロードをスキップする。

```bash
CHECK_DIRS="/media/videos,/media/archive"
```

- `DOWNLOAD_DIR`、ドメイン別ルールのディレクトリ、`CHECK_DIRS` のすべてが検索対象
- ファイル名の拡張子とケースを無視してマッチ
- スキップ時はステータス `skipped` で記録される

### ダウンロードの永続化

ダウンロード状態は `DOWNLOAD_DIR/.dlrelay/downloads.json` に自動保存される。サーバーを再起動しても、完了済みのダウンロード履歴が保持され、未完了のダウンロードは自動的に再開される。

## API

認証が有効な場合、書き込み系のエンドポイントには `X-API-Key` ヘッダーが必要。

### ダウンロード管理

```
POST   /api/downloads              ダウンロード開始
GET    /api/downloads              一覧取得
GET    /api/downloads/:id          進捗取得
GET    /api/downloads/:id/file     完了済みファイルの取得（プレビュー/ダウンロード）
POST   /api/downloads/:id/retry    失敗/キャンセル済みのリトライ
DELETE /api/downloads/:id          キャンセル/削除
```

`POST /api/downloads` のリクエスト例:

```json
{
  "url": "https://cdn.example.com/video.mp4",
  "audio_url": "https://cdn.example.com/audio.m4a",
  "fallback_url": "https://fallback-url.com/video.mp4",
  "filename": "video.mp4",
  "directory": "subfolder",
  "method": "ytdlp",
  "quality": "bestvideo+bestaudio/best",
  "page_url": "https://example.com/watch",
  "headers": {
    "Cookie": "session=abc123",
    "Referer": "https://example.com/watch"
  }
}
```

| フィールド | 必須 | 説明 |
|---|---|---|
| `url` | Yes | ダウンロードURL |
| `audio_url` | No | 音声URL（DASH用。指定時は映像+音声を別々にDLしFFmpegでmux） |
| `fallback_url` | No | yt-dlp失敗時のフォールバックURL |
| `filename` | No | 保存ファイル名 |
| `directory` | No | サブディレクトリ |
| `method` | No | `ytdlp` を指定するとyt-dlpで処理 |
| `quality` | No | yt-dlpのフォーマット指定 |
| `page_url` | No | 元ページのURL（ダッシュボードでリンク表示に使用） |
| `headers` | No | ダウンロード時に送信するHTTPヘッダー |

### FFmpeg変換

```
POST   /api/convert                変換開始
GET    /api/convert/:id            変換進捗
DELETE /api/convert/:id            変換キャンセル
```

### その他

```
GET    /api/health                 サーバー状態
GET    /api/extension.zip          拡張機能（サーバーURL設定済み）
POST   /api/probe                  ffprobe実行
GET    /api/codecs                 利用可能コーデック一覧
GET    /api/formats                利用可能フォーマット一覧
```

## ビルド

```bash
make build-server    # サーバーバイナリ → bin/dlrelay-server
make docker-build    # Dockerイメージ
make test            # テスト
make pack-extension  # 拡張zipパッケージ → bin/dlrelay-extension.zip
```

## ドキュメント

- [ansible-nas統合ガイド](docs/ansible-nas-integration.md)
