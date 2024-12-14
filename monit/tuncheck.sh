#!/usr/bin/env bash
set -u
r="${1:-"tr"}"
curl -sx socks5h://127.0.0.1:19645 --connect-timeout 2 --retry 3 --retry-delay 1 -m 22 \
  --proxy-user "o9r1-proton-${r}:o9r1-proton-${r}" -Lo /dev/null --retry-all-errors \
  https://github.com/raven428/container-images/releases/download/000/prettier-2_5_1.tar.xz
res="$?"
echo "[${r}] res [${res}]"
exit "$res"
