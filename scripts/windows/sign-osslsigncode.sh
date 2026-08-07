#!/bin/bash
# Authenticode-sign Windows PE/MSI on macOS with SafeNet eToken + osslsigncode.
#
# Required env (PIN is never stored in repo files):
#   SAFENET_PIN=...
#   SIGN_PKCS11_MODULE=/path/to/pkcs11.dylib
#   SIGN_PKCS11_KEY='pkcs11:token=...;object=...;type=private'
#   SIGN_CERT_FILE=certs/signing_chain.pem
# Optional:
#   SIGN_TIMESTAMP_URL=...
#   COMPANY_URL=...
#   PRODUCT_NAME=VPNToris
#
# Usage:
#   SAFENET_PIN=**** ./scripts/windows/sign-osslsigncode.sh path/to.exe [more.exe ...]
#   SAFENET_PIN=**** ./scripts/windows/sign-osslsigncode.sh --desc "VPNToris Helper" helper.exe
#
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

SIGN_PKCS11_MODULE=${SIGN_PKCS11_MODULE:-/Library/Frameworks/eToken.framework/Versions/Current/libeToken.dylib}
if [[ -z ${SIGN_CERT_FILE:-} && -f $ROOT_DIR/certs/signing_chain.pem ]]; then
  SIGN_CERT_FILE=$ROOT_DIR/certs/signing_chain.pem
fi
SIGN_TIMESTAMP_URL=${SIGN_TIMESTAMP_URL:-http://timestamp.digicert.com}
COMPANY_URL=${COMPANY_URL:-}
PRODUCT_NAME=${PRODUCT_NAME:-VPNToris}

DESC_OVERRIDE=""
FILES=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --desc)
      DESC_OVERRIDE=${2:-}
      shift 2
      ;;
    -*)
      echo "unknown flag: $1" >&2
      exit 1
      ;;
    *)
      FILES+=("$1")
      shift
      ;;
  esac
done

if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "usage: SAFENET_PIN=... $0 [--desc name] artifact.exe [artifact.exe ...]" >&2
  exit 1
fi

if ! command -v osslsigncode >/dev/null 2>&1; then
  echo "[Sign] error: osslsigncode not found (brew install osslsigncode)" >&2
  exit 1
fi
if [[ -z ${SAFENET_PIN:-} ]]; then
  echo "[Sign] error: SAFENET_PIN env var is required" >&2
  exit 1
fi
if [[ -z ${SIGN_PKCS11_MODULE:-} || ! -f $SIGN_PKCS11_MODULE ]]; then
  echo "[Sign] error: SIGN_PKCS11_MODULE not found: ${SIGN_PKCS11_MODULE:-<unset>}" >&2
  exit 1
fi
if [[ -z ${SIGN_CERT_FILE:-} || ! -f $SIGN_CERT_FILE ]]; then
  echo "[Sign] error: SIGN_CERT_FILE not found: ${SIGN_CERT_FILE:-<unset>}" >&2
  exit 1
fi
if [[ -z ${SIGN_PKCS11_KEY:-} ]]; then
  echo "[Sign] error: SIGN_PKCS11_KEY is not set" >&2
  echo "       example: pkcs11:token=<token>;object=<object>;type=private" >&2
  exit 1
fi

SIGN_ENGINE=${SIGN_PKCS11_ENGINE:-}
if [[ -z $SIGN_ENGINE ]]; then
  for candidate in \
    "$(brew --prefix libp11 2>/dev/null)/lib/engines-3/pkcs11.dylib" \
    "$(brew --prefix libp11 2>/dev/null)/lib/engines-3/pkcs11.so" \
    "$(brew --prefix openssl@3 2>/dev/null)/lib/engines-3/pkcs11.dylib" \
    "$(brew --prefix openssl@3 2>/dev/null)/lib/engines-3/pkcs11.so" \
    "/usr/local/lib/engines-3/pkcs11.dylib" \
    "/opt/homebrew/lib/engines-3/pkcs11.dylib"; do
    if [[ -n $candidate && -f $candidate ]]; then
      SIGN_ENGINE=$candidate
      break
    fi
  done
fi
if [[ -z $SIGN_ENGINE || ! -f $SIGN_ENGINE ]]; then
  echo "[Sign] error: PKCS#11 OpenSSL engine (libp11) not found" >&2
  echo "       macOS: brew install libp11" >&2
  exit 1
fi

echo "[Sign] Engine:  $SIGN_ENGINE"
echo "[Sign] Module:  $SIGN_PKCS11_MODULE"
echo "[Sign] Certs:   $SIGN_CERT_FILE"
echo "[Sign] TS:      $SIGN_TIMESTAMP_URL"
echo "[Sign] Product: $PRODUCT_NAME"

sign_one() {
  local IN=$1
  local DESC=$2
  if [[ ! -f $IN ]]; then
    echo "[Sign]   skip (missing): $(basename "$IN")"
    return 0
  fi
  local TMP="${IN}.signing_tmp"
  echo "[Sign]   $(basename "$IN") ..."
  local -a extra=()
  if [[ -n $COMPANY_URL ]]; then
    extra+=(-i "$COMPANY_URL")
  fi
  osslsigncode sign \
    -pkcs11engine "$SIGN_ENGINE" \
    -pkcs11module "$SIGN_PKCS11_MODULE" \
    -key "${SIGN_PKCS11_KEY};pin-value=${SAFENET_PIN}" \
    -certs "$SIGN_CERT_FILE" \
    -ts "$SIGN_TIMESTAMP_URL" \
    -h sha256 \
    -n "$DESC" \
    "${extra[@]+"${extra[@]}"}" \
    -in "$IN" \
    -out "$TMP"
  mv "$TMP" "$IN"
  echo "[Sign]   OK: $(basename "$IN")"
}

for input in "${FILES[@]}"; do
  base=$(basename "$input")
  desc=${DESC_OVERRIDE:-$PRODUCT_NAME}
  case "$base" in
    *.msi) desc="$PRODUCT_NAME Installer" ;;
    *tray*) desc="$PRODUCT_NAME Tray" ;;
    *helper*) desc="$PRODUCT_NAME Native Helper" ;;
    *service*) desc="$PRODUCT_NAME Service" ;;
    *ctl*) desc="$PRODUCT_NAME CLI" ;;
    *vpntorisd*|*agent*) desc="$PRODUCT_NAME" ;;
  esac
  sign_one "$input" "$desc"
done

echo "[Sign] done"
