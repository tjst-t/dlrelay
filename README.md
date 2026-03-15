# vdh-relay

ブラウザで検出した動画をNASにダウンロードするシステム。

拡張機能が動画URL（HLS/DASH/直接ストリーム）を自動検出し、NAS上のサーバーでダウンロードを実行する。

## アーキテクチャ

```
クライアントPC                          NAS
┌─────────────────┐    HTTPS     ┌──────────────────────┐
│  VDH Relay 拡張  │              │  Caddy (リバースプロキシ) │
│  ├─ 動画検出      │              │      ↓                │
│  ├─ 画質選択      │ ──────────→ │  vdh-relay-server     │
│  └─ DLボタン     │              │  ├─ REST API          │
│                  │              │  ├─ HTTPダウンローダー   │
└─────────────────┘              │  ├─ HLS/DASHダウンローダー│
                                 │  └─ FFmpeg (変換/結合)  │
                                 │      ↓                │
                                 │  /tank/downloads/     │
                                 └──────────────────────┘
```

## コンポーネント

| コンポーネント | 説明 |
|---|---|
| **vdh-relay-server** | NAS上のDockerコンテナ。ダウンロード実行・FFmpeg変換・進捗管理 |
| **VDH Relay 拡張** | ブラウザ拡張。動画を自動検出し、サーバーにダウンロードを指示 |
| **vdh-relay (クライアント)** | **オプション:** VDH互換のNative Messaging Host。既存VDHを使い続ける場合のみ必要 |

## クイックスタート

### 1. サーバー起動

```bash
# Docker
docker build -t vdh-relay-server .
docker run -d \
  -p 8090:8090 \
  -v /tank/downloads/vdh:/downloads \
  -e MAX_CONCURRENT=3 \
  vdh-relay-server

# または直接
make serve
```

### 2. 拡張機能インストール

サーバーのWebUI (`http://your-nas:8090`) にアクセスし、「Extension をダウンロード」ボタンをクリック。サーバーURLが設定済みの拡張がダウンロードされる。

**Chrome / Edge:**
1. zipをダウンロードし展開
2. `chrome://extensions` を開き「デベロッパーモード」ON
3. 「パッケージ化されていない拡張機能を読み込む」で展開フォルダを選択

**Firefox:**
1. zipをダウンロード
2. `about:debugging#/runtime/this-firefox` を開く
3. 「一時的なアドオンを読み込む」でzip内のmanifest.jsonを選択

### 3. 使い方

1. 動画のあるページを開く → 拡張が自動検出しバッジに数を表示
2. 拡張のポップアップを開く → 検出された動画一覧が表示
3. HLS/DASHは画質を選択可能
4. 「Download」ボタンでNASへダウンロード開始

## 拡張機能の検出方式

| 方式 | 説明 |
|---|---|
| **webRequest** | ネットワーク監視で動画MIME/拡張子を検出 |
| **DOM** | `<video>`/`<audio>`タグのsrc属性を収集 |
| **HLS解析** | M3U8マスタープレイリストから画質バリアントを抽出 |
| **DASH解析** | MPDマニフェストから映像+音声の組み合わせを抽出 |
| **blob: フック** | MediaSource APIフックで元URLを逆引き |
| **サイト固有** | Twitter, Instagram, TikTok, YouTube, Reddit, ニコニコのAPI傍受 |

## ビルド

```bash
make build-all      # サーバー + クライアント
make docker-build   # Dockerイメージ
make test           # テスト
make pack-extension # 拡張zipパッケージ
```

## サーバー設定 (環境変数)

| 変数 | デフォルト | 説明 |
|---|---|---|
| `LISTEN_ADDR` | `:8090` | リッスンアドレス |
| `DOWNLOAD_DIR` | `/downloads` | ダウンロード保存先 |
| `TEMP_DIR` | システム依存 | 一時ファイル |
| `MAX_CONCURRENT` | `3` | 最大同時ダウンロード数 |
| `BIN_DIR` | (なし) | クライアントバイナリ配布用 |
| `EXTENSION_DIR` | (なし) | 拡張機能ソースディレクトリ (WebUIからのDL用) |
| `API_KEY` | (なし) | API認証キー（設定時、書き込みAPIに `X-API-Key` ヘッダーが必要） |
| `DOWNLOAD_RULES` | (なし) | ドメイン別ダウンロード先ルール（後述） |

### ドメイン別ダウンロード先 (`DOWNLOAD_RULES`)

ドメインごとに異なるディレクトリにダウンロードできる。

```bash
DOWNLOAD_RULES="youtube.com:/downloads/youtube,twitter.com:/downloads/twitter,nicovideo.jp:/downloads/niconico"
```

| 設定 | 動作 |
|---|---|
| `youtube.com:/downloads/youtube` | `youtube.com` および `www.youtube.com` 等のサブドメインからのダウンロードを `/downloads/youtube` に保存 |
| `twitter.com:/downloads/twitter` | `twitter.com` からのダウンロードを `/downloads/twitter` に保存 |

- カンマ区切りで複数ルールを指定
- サブドメインは自動マッチ（`youtube.com` は `www.youtube.com` にもマッチ）
- どのルールにもマッチしない場合は `DOWNLOAD_DIR` に保存
- リクエストの `directory` フィールドが指定されていれば、ルールで決まったディレクトリ配下のサブディレクトリとして使用

Docker での使用例:

```bash
docker run -d \
  -p 8090:8090 \
  -v /tank/downloads/vdh:/downloads \
  -v /tank/downloads/youtube:/downloads/youtube \
  -e DOWNLOAD_RULES="youtube.com:/downloads/youtube" \
  -e MAX_CONCURRENT=3 \
  vdh-relay-server
```

### ダウンロードの永続化

ダウンロード状態は `DOWNLOAD_DIR/.vdh-relay/downloads.json` に自動保存される。サーバーを再起動しても、完了済みのダウンロード履歴が保持され、未完了のダウンロードは自動的に再開される。

## API

### ダウンロード管理

```
POST   /api/downloads              ダウンロード開始
GET    /api/downloads               一覧取得
GET    /api/downloads/:id           進捗取得
GET    /api/downloads/:id/file      完了済みファイルの取得（プレビュー/ダウンロード）
DELETE /api/downloads/:id           キャンセル
```

`POST /api/downloads` のリクエスト例:

```json
{
  "url": "https://cdn.example.com/video.mp4",
  "audio_url": "https://cdn.example.com/audio.m4a",
  "filename": "video.mp4",
  "headers": {
    "Cookie": "session=abc123",
    "Referer": "https://example.com/watch"
  }
}
```

`audio_url` を指定すると、映像と音声を別々にダウンロードしFFmpegでmuxする（DASH用）。

### FFmpeg変換

```
POST   /api/convert             変換開始
GET    /api/convert/:id         変換進捗
DELETE /api/convert/:id         変換キャンセル
```

### その他

```
GET    /api/health              サーバー状態
GET    /api/extension.zip       拡張機能 (サーバーURL設定済み)
POST   /api/probe               ffprobe実行
GET    /api/codecs              利用可能コーデック一覧
GET    /api/formats             利用可能フォーマット一覧
```

## VDH CoApp 互換クライアント (オプション)

既存のVideoDownloadHelperを使い続ける場合のみ必要。拡張機能だけで十分な場合は不要。

```bash
# サーバーのWebUIから1行インストール
curl -fsSL http://your-nas:8090/api/install.sh | bash

# または手動
make build-client
./bin/vdh-relay install
mkdir -p ~/.config/vdh-relay
echo 'server_url = "http://your-nas:8090"' > ~/.config/vdh-relay/config.toml
```

## ドキュメント

- [ansible-nas統合ガイド](docs/ansible-nas-integration.md)
