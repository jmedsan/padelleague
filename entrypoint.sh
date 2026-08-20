#!/bin/sh
if [ -n "$PB_ADMIN_EMAIL" ] && [ -n "$PB_ADMIN_PASSWORD" ]; then
    ./padelleague superuser upsert "$PB_ADMIN_EMAIL" "$PB_ADMIN_PASSWORD" 2>/dev/null
fi
exec ./padelleague serve --http=0.0.0.0:8090
