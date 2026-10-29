#!/bin/sh
set -e

echo "[GO] Starting Home Cooking Server..."

if [ -x /app/homeCookingServer ]; then
    exec /app/homeCookingServer
elif [ -x /app/app/homeCookingServer ]; then
    exec /app/app/homeCookingServer
fi

echo "[ERROR] Executable not found. Expected /app/homeCookingServer or /app/app/homeCookingServer." >&2
echo "[ERROR] Place the Linux homeCookingServer binary in the mounted host directory and ensure it is executable." >&2
exit 127
