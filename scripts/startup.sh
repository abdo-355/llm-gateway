#!/bin/sh
set -e

if [ -n "$GCP_SA_KEY_JSON" ]; then
    CRED_PATH="${GCP_CREDENTIALS_PATH:-/app/secrets/gcp-sa.json}"
    mkdir -p "$(dirname "$CRED_PATH")"
    printf '%s' "$GCP_SA_KEY_JSON" > "$CRED_PATH"
    chmod 600 "$CRED_PATH"
    export GOOGLE_APPLICATION_CREDENTIALS="$CRED_PATH"
    echo "[startup] GCP credentials written to $CRED_PATH"
else
    echo "[startup] GCP_SA_KEY_JSON not set, skipping credentials bootstrap"
fi

exec ./gateway "$@"