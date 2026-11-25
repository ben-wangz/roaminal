#!/usr/bin/env bash
# Run repository contracts against a throwaway loopback PostgreSQL with TLS.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
pgbin=$(pg_config --bindir)
work=$(mktemp -d)
data="$work/data"
port=${ROAMINAL_TEST_POSTGRES_PORT:-55461}
cleanup() {
  "$pgbin/pg_ctl" -D "$data" -m immediate -w stop >/dev/null 2>&1 || true
  rm -rf -- "$work"
}
trap cleanup EXIT

umask 077
"$pgbin/initdb" -D "$data" --encoding=UTF8 --no-locale --username=fixture_admin --auth-local=trust --auth-host=scram-sha-256 >/dev/null
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=localhost \
  -addext 'subjectAltName=DNS:localhost' -keyout "$data/server.key" -out "$data/server.crt" >/dev/null 2>&1
printf '\nssl = on\nlisten_addresses = %s\n' "'127.0.0.1'" >> "$data/postgresql.conf"
printf 'local all all trust\nhostssl all all 127.0.0.1/32 scram-sha-256\n' > "$data/pg_hba.conf"
"$pgbin/pg_ctl" -D "$data" -l "$work/postgres.log" -o "-p $port -k $work" -w start >/dev/null
"$pgbin/psql" -X -h "$work" -p "$port" -U fixture_admin -d postgres -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
CREATE ROLE roaminal_fixture LOGIN PASSWORD 'disposable-fixture-only';
CREATE DATABASE roaminal_fixture OWNER roaminal_fixture;
\connect roaminal_fixture
CREATE SCHEMA cli_cutover AUTHORIZATION roaminal_fixture;
SQL
export ROAMINAL_TEST_POSTGRES_DISPOSABLE=1
export ROAMINAL_TEST_POSTGRES_DSN="postgresql://roaminal_fixture:disposable-fixture-only@localhost:$port/roaminal_fixture?sslmode=verify-full&sslrootcert=$data/server.crt"
export ROAMINAL_TEST_POSTGRES_CLI_DSN="${ROAMINAL_TEST_POSTGRES_DSN}&search_path=cli_cutover"
go -C "$root/backend" test -count=1 -timeout=4m ./internal/persistence/db ./cmd/roaminal -run '^TestPostgres' "$@"
