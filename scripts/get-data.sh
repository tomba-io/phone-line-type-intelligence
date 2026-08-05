#!/bin/sh
# Fetches the public source files needed to build the linetype tables.
#
# GEOGRAPHIC BLOCKING: reports.nanpa.com sits behind Imperva, which refuses
# requests from a number of countries outright. If every attempt below returns
# 403 or times out, route through a proxy:
#
#   cp .proxyrc.example .proxyrc   # edit with your credentials
#   sh get-data.sh
set -e

cd "$(dirname "$0")"

DEST=${DEST:-../../_build/linetype}
CO_URL=${CO_URL:-https://reports.nanpa.com/public/CoCodeAssignment_Utilized_AllStates_Public.zip}
BLOCK_URL=${BLOCK_URL:-https://reports.nanpa.com/public/ThousandsBlockAssignment_All_Augmented.zip}
UA=${UA:-'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126 Safari/537.36'}

# Load proxy settings from .proxyrc if present (gitignored, holds credentials).
if [ -f .proxyrc ]; then
    . ./.proxyrc
    echo "Loaded proxy settings from .proxyrc."
elif [ -n "$HTTPS_PROXY$https_proxy" ]; then
    echo "Using the proxy from HTTPS_PROXY."
fi

mkdir -p "$DEST"

fetch() {
    url=$1; out=$2; tmp="$out.part"
    printf '  %s\n' "$url"
    code=$(curl -sSL -A "$UA" --max-time 180 --retry 2 -o "$tmp" -w '%{http_code}' "$url" 2>/dev/null || echo 000)
    if [ "$code" != "200" ]; then
        rm -f "$tmp"
        if [ "$code" = "403" ]; then
            echo "    FAILED: HTTP 403. Imperva is refusing this network." >&2
            echo "            Copy .proxyrc.example to .proxyrc, fill it in, re-run." >&2
        else
            echo "    FAILED: HTTP $code (rejected or timed out)" >&2
        fi
        return 1
    fi
    size=$(wc -c < "$tmp")
    if [ "$size" -lt 10000 ]; then
        echo "    FAILED: got $size bytes, far too small" >&2; rm -f "$tmp"; return 1
    fi
    if head -c 1000 "$tmp" | grep -qi '<html\|Incapsula\|Access Denied\|Request unsuccessful'; then
        echo "    FAILED: server returned an HTML challenge page, not data" >&2; rm -f "$tmp"; return 1
    fi
    mv "$tmp" "$out"; echo "    OK: $size bytes -> $out"
}

ok=0
echo "Central office codes (NXX level):"
fetch "$CO_URL" "$DEST/cocodes.zip" && ok=$((ok + 1)) || true

echo "Thousands blocks (block level, overrides NXX):"
fetch "$BLOCK_URL" "$DEST/blocks.zip" && ok=$((ok + 1)) || true

# Mexico IFT — JSF form requiring ViewState POST
fetch_mx() {
    page="https://sns.ift.org.mx/sns-frontend/planes-numeracion/descarga-publica.xhtml"
    jar=$(mktemp)
    echo "Mexico, Plan Nacional de Numeracion:"
    if ! curl -fsSL -A "$UA" --max-time 90 -c "$jar" -o "$DEST/sns.html" "$page"; then
        echo "    FAILED: cannot reach sns.ift.org.mx" >&2; rm -f "$jar"; return 1
    fi
    vs=$(sed -n 's/.*name="javax\.faces\.ViewState"[^>]*value="\([^"]*\)".*/\1/p' \
         "$DEST/sns.html" | head -1 | sed 's/&amp;/\&/g; s/&lt;/</g; s/&gt;/>/g')
    if [ -z "$vs" ]; then
        echo "    FAILED: no ViewState on the page" >&2; rm -f "$jar"; return 1
    fi
    if ! curl -fsS -A "$UA" --max-time 300 -b "$jar" -e "$page" \
        --data-urlencode "FORM_planes=FORM_planes" \
        --data-urlencode "FORM_planes:BTN_planPublico1=" \
        --data-urlencode "javax.faces.ViewState=$vs" \
        -o "$DEST/mx_pnn.zip" "$page"; then
        echo "    FAILED: the download POST was rejected" >&2; rm -f "$jar"; return 1
    fi
    rm -f "$jar" "$DEST/sns.html"
    size=$(wc -c < "$DEST/mx_pnn.zip")
    if [ "$size" -lt 100000 ]; then
        echo "    FAILED: got $size bytes, too small" >&2; rm -f "$DEST/mx_pnn.zip"; return 1
    fi
    echo "    OK: $size bytes -> $DEST/mx_pnn.zip"
}
fetch_mx || true

echo "Canadian CO codes (CNAC):"
fetch "https://crtc.gc.ca/public/cisc/cnac/COCodeStatus_ALL.csv" "$DEST/ca_cocodes.csv" || true

for z in "$DEST"/*.zip; do
    [ -f "$z" ] || continue
    echo "Unpacking $z"
    unzip -o -q -d "$DEST" "$z" || echo "  WARNING: unzip failed for $z" >&2
done

echo
if [ "$ok" -eq 2 ]; then
    echo "Both NANPA files downloaded."
else
    cat <<'MANUAL'
=== DOWNLOAD FAILED ===

One or both sources refused the request. The usual cause is Imperva bot/geo
protection at reports.nanpa.com. Two ways round it:

  a) Route through a proxy:
       cp cmd/buildlinetype/.proxyrc.example cmd/buildlinetype/.proxyrc
       # Edit .proxyrc with your proxy credentials
       bash cmd/buildlinetype/get-data.sh

  b) Download in a browser from:
       https://www.nanpa.com/reports/co-code-reports/cocodes_assign
       https://www.nanpa.com/reports/thousands-block-reports/region
     Save into _build/linetype/ and unzip.
MANUAL
fi
