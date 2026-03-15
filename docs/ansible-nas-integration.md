# ansible-nas統合ガイド

dlrelay-serverをansible-nasの既存アーキテクチャに統合する手順。

## 1. services.yml にイメージ追加

```yaml
docker_services:
  # ... 既存サービス
  dlrelay: v0.1.0
```

## 2. docker-composeテンプレート

`roles/services/templates/dlrelay-docker-compose.yml.j2`:

```yaml
services:
  dlrelay-server:
    image: ghcr.io/tjst-t/dlrelay-server:{{ docker_services.dlrelay }}
    container_name: dlrelay-server
    restart: unless-stopped
    environment:
      - LISTEN_ADDR=:8090
      - DOWNLOAD_DIR=/downloads
      - MAX_CONCURRENT=3
      - TZ=Asia/Tokyo
    volumes:
      - "{{ dlrelay_download_dir }}:/downloads"
    networks:
      - {{ services_docker_network }}
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://localhost:8090/api/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3

networks:
  {{ services_docker_network }}:
    external: true
```

## 3. reverse-proxy.yml にルート追加

```yaml
caddy_proxy_entries:
  # ... 既存ルート
  - subdomain: dl
    upstream: dlrelay-server:8090
```

→ `https://dl.nas.tjstkm.net` でアクセス可能に（Authelia認証付き）

## 4. ストレージ

| 用途 | パス |
|---|---|
| ダウンロード先 | `/tank/downloads` |
| 一時ファイル | `/fast/docker/dlrelay/tmp/` |

## 5. 変数定義

`roles/services/defaults/main.yml`:

```yaml
dlrelay_enabled: true
dlrelay_download_dir: /tank/downloads
dlrelay_port: 8090
```

## 6. 認証

Autheliaで保護される。API_KEY環境変数を設定すると、書き込みAPIに `X-API-Key` ヘッダーが必要になる。

## 7. CI/CD

- GitHub Container Registry (ghcr.io) にDockerイメージをpush
- タグベースのリリース（`v0.1.0` → `ghcr.io/tjst-t/dlrelay-server:v0.1.0`）
- ansible-nasの `services.yml` でバージョン管理
