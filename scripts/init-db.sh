#!/bin/sh
set -e

echo "Running initial database migrations..."
for file in $(ls /migrations/*_*.up.sql | sort); do
    echo "Applying $file..."
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$file"
done
echo "All migrations applied successfully!"
