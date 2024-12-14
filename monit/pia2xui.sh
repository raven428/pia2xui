#!/usr/bin/env bash
set -u
r="${1:-"santiago"}"
t="${2:-"wg-proton-cl27"}"
my_bin="$(readlink -f "$0")"
my_dir="$(dirname "${my_bin}")"
exec 9> "${my_bin%.*}.lock"
flock -w 11 9
# shellcheck disable=1091
source "${my_dir}"/creds.sh
# shellcheck disable=2154
"${my_dir}"/pia2xui \
  -tag "${t}" \
  -region "${r}" \
  -username "${l}" \
  -password "${p}" \
  -db /etc/x-ui/x-ui.db \
  -cert "${my_dir}"/ca.rsa.4096.crt
