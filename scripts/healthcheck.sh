#!/usr/bin/env bash

set -Eeuo pipefail

readonly APP_URL="${FUT_APP_URL:-https://fut.devarthur.com.br}"

curl --fail --silent --show-error --max-time 10 "$APP_URL/api/healthz" | grep -q '"status":"ok"'
