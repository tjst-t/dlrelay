#!/bin/sh
# yt-dlp is only pinned at image build time; sites like SpankBang rotate
# anti-bot defenses faster than dlrelay releases (and thus image rebuilds)
# happen. Decouple yt-dlp's version from the image lifecycle by updating it
# in the background, independent of when the image was built.
set -e

YTDLP_UPDATE_INTERVAL="${YTDLP_UPDATE_INTERVAL:-86400}"

update_ytdlp_loop() {
    while true; do
        if pip3 install -q -U --break-system-packages "yt-dlp[default,curl-cffi]"; then
            echo "[yt-dlp-update] updated to $(yt-dlp --version 2>/dev/null || echo unknown)"
        else
            echo "[yt-dlp-update] update failed, will retry in ${YTDLP_UPDATE_INTERVAL}s" >&2
        fi
        sleep "$YTDLP_UPDATE_INTERVAL"
    done
}

if [ "${YTDLP_AUTO_UPDATE:-1}" != "0" ] && command -v yt-dlp >/dev/null 2>&1; then
    update_ytdlp_loop &
fi

exec dlrelay-server "$@"
