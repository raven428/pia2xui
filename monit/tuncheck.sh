#!/usr/bin/env bash
set -u
r="${1:-"warp"}"
curl -sx socks5h://127.0.0.1:19645 --connect-timeout 2 --retry 3 --retry-delay 1 -m 5 \
  --proxy-user "o9r1-proton-${r}:o9r1-proton-${r}" -Lo /dev/null --retry-all-errors \
  https://www.gstatic.com/generate_204
# https://connectivitycheck.platform.hicloud.com/generate_204
# https://connectivitycheck.gstatic.com/generate_204
# https://captiveportal.kuketz.de/generate_204
# https://play.googleapis.com/generate_204
# https://google.com/generate_204 - captcha
res="$?"
echo "[${r}] res [${res}]"
exit "$res"
