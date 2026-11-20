
#!/usr/bin/env bash
set -euo pipefail
umask 077

# ============================================================
# Configuration
# ============================================================

STACK="$HOME/binder-web"
PORT=7070

TOOLS="$STACK/tools"
DOWNLOADS="$STACK/downloads"
DB="$STACK/mariadb-server"
LIB_ENV="$STACK/db-env"

SOCKET="$STACK/run/mariadb.sock"
ROOT_CONFIG="$STACK/root.cnf"
LOGIN_FILE="$STACK/login.txt"
WEB_PID="$STACK/run/phpmyadmin.pid"

MICRO="$TOOLS/bin/micromamba"
PHP="$TOOLS/frankenphp"

export MAMBA_ROOT_PREFIX="$STACK/mamba-root"

fail() {
    echo
    echo "[ERROR] $*" >&2
    exit 1
}

case "$(uname -m)" in
    x86_64)
        CONDA_PLATFORM="linux-64"
        PHP_ARCH="x86_64"
        ;;
    *)
        fail "This installer currently supports Linux x86_64 only."
        ;;
esac

for tool in wget tar python; do
    command -v "$tool" >/dev/null ||
        fail "Required command is missing: $tool"
done

mkdir -p \
    "$TOOLS" \
    "$DOWNLOADS" \
    "$STACK/run" \
    "$STACK/logs" \
    "$STACK/data" \
    "$STACK/pma-tmp"

chmod 700 "$STACK/pma-tmp"

# Protect existing data if its administrator credentials are missing.
if [ -d "$STACK/data/mysql" ] && [ ! -s "$ROOT_CONFIG" ]; then
    fail "Existing database found, but $ROOT_CONFIG is missing.
Stopping without changing your database."
fi

# ============================================================
# 1. Download micromamba
# ============================================================

if [ ! -x "$MICRO" ]; then
    echo "=== Downloading micromamba ==="

    wget --https-only \
        -O "$DOWNLOADS/micromamba.tar.bz2" \
        "https://micro.mamba.pm/api/micromamba/$CONDA_PLATFORM/latest"

    tar -xjf "$DOWNLOADS/micromamba.tar.bz2" \
        -C "$TOOLS" \
        bin/micromamba
else
    echo "=== Reusing micromamba ==="
fi

# ============================================================
# 2. Install local compatibility libraries
# ============================================================

echo "=== Preparing local MariaDB libraries ==="

if [ -d "$LIB_ENV/conda-meta" ]; then
    MAMBA_ACTION="install"
else
    MAMBA_ACTION="create"
fi

"$MICRO" "$MAMBA_ACTION" \
    --yes \
    --prefix "$LIB_ENV" \
    --override-channels \
    --channel conda-forge \
    libaio liburing libxcrypt ncurses

# ============================================================
# 3. Download the actual MariaDB server
# ============================================================

if [ ! -x "$DB/bin/mariadbd" ] &&
   [ ! -x "$DB/sbin/mariadbd" ]; then

    echo "=== Finding an available MariaDB 11.4 release ==="

    wget --https-only \
        -O "$DOWNLOADS/mariadb-archive.html" \
        "https://archive.mariadb.org/"

    RELEASE="$(
        grep -oE 'mariadb-11\.4\.[0-9]+' \
            "$DOWNLOADS/mariadb-archive.html" |
        sort -Vu |
        tail -n 1
    )"

    [ -n "$RELEASE" ] ||
        fail "Could not discover a MariaDB 11.4 release."

    ARCHIVE="$RELEASE-linux-systemd-x86_64.tar.gz"
    URL="https://archive.mariadb.org/$RELEASE/bintar-linux-systemd-x86_64/$ARCHIVE"

    echo "=== Downloading $RELEASE server ==="

    wget --https-only --continue \
        -O "$DOWNLOADS/$ARCHIVE" \
        "$URL"

    mkdir -p "$DB"

    tar -xzf "$DOWNLOADS/$ARCHIVE" \
        -C "$DB" \
        --strip-components=1
else
    echo "=== Reusing MariaDB server binaries ==="
fi

export PATH="$DB/bin:$DB/sbin:$DB/scripts:$PATH"
export LD_LIBRARY_PATH="$DB/lib:$DB/lib/private:$LIB_ENV/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"

if [ -x "$DB/bin/mariadbd" ]; then
    SERVER="$DB/bin/mariadbd"
elif [ -x "$DB/sbin/mariadbd" ]; then
    SERVER="$DB/sbin/mariadbd"
else
    fail "MariaDB server executable was not found."
fi

CLIENT="$DB/bin/mariadb"

[ -x "$CLIENT" ] ||
    fail "MariaDB command-line client was not found."

if [ -x "$DB/scripts/mariadb-install-db" ]; then
    INSTALL_DB="$DB/scripts/mariadb-install-db"
elif [ -x "$DB/bin/mariadb-install-db" ]; then
    INSTALL_DB="$DB/bin/mariadb-install-db"
else
    fail "mariadb-install-db was not found."
fi

echo "=== Checking MariaDB executables ==="

"$SERVER" --version
"$CLIENT" --version

# ============================================================
# 4. Download standalone PHP
# ============================================================

if [ ! -x "$PHP" ]; then
    echo "=== Downloading standalone PHP ==="

    wget --https-only \
        -O "$PHP.download" \
        "https://github.com/php/frankenphp/releases/latest/download/frankenphp-linux-$PHP_ARCH"

    chmod 700 "$PHP.download"
    mv "$PHP.download" "$PHP"
else
    echo "=== Reusing standalone PHP ==="
fi

echo "=== Checking PHP extensions ==="

"$PHP" php-cli -r '
$required = [
    "mysqli",
    "mbstring",
    "session",
    "ctype",
    "json",
    "xml"
];

$missing = [];

foreach ($required as $extension) {
    if (!extension_loaded($extension)) {
        $missing[] = $extension;
    }
}

if ($missing) {
    fwrite(
        STDERR,
        "Missing PHP extensions: " .
        implode(", ", $missing) .
        PHP_EOL
    );
    exit(1);
}

echo "PHP extension check passed.", PHP_EOL;
'

# ============================================================
# 5. Download phpMyAdmin
# ============================================================

if [ ! -f "$STACK/phpmyadmin/index.php" ]; then
    echo "=== Downloading phpMyAdmin ==="

    wget --https-only \
        -O "$DOWNLOADS/phpmyadmin.tar.gz" \
        "https://www.phpmyadmin.net/downloads/phpMyAdmin-latest-all-languages.tar.gz"

    mkdir -p "$STACK/phpmyadmin"

    tar -xzf "$DOWNLOADS/phpmyadmin.tar.gz" \
        -C "$STACK/phpmyadmin" \
        --strip-components=1
else
    echo "=== Reusing phpMyAdmin ==="
fi

# ============================================================
# 6. Initialize a new database only if necessary
# ============================================================

NEW_DATABASE=0

if [ ! -d "$STACK/data/mysql" ]; then
    echo "=== Initializing a new MariaDB database ==="

    "$INSTALL_DB" \
        --no-defaults \
        --basedir="$DB" \
        --datadir="$STACK/data" \
        --auth-root-authentication-method=normal \
        --skip-test-db

    NEW_DATABASE=1
else
    echo "=== Preserving existing database ==="
fi

database_ready() {
    if [ "$NEW_DATABASE" -eq 1 ]; then
        "$CLIENT" \
            --no-defaults \
            --protocol=socket \
            --socket="$SOCKET" \
            --user=root \
            --connect-timeout=2 \
            --execute="SELECT 1;" \
            >/dev/null 2>&1
    else
        "$CLIENT" \
            --defaults-file="$ROOT_CONFIG" \
            --protocol=socket \
            --connect-timeout=2 \
            --execute="SELECT 1;" \
            >/dev/null 2>&1
    fi
}

# ============================================================
# 7. Start MariaDB if it is not already running
# ============================================================

if database_ready; then
    echo "=== MariaDB is already running ==="
else
    # Avoid launching a second server against the same data directory.
    if [ -s "$STACK/run/mariadb.pid" ]; then
        DB_PID="$(cat "$STACK/run/mariadb.pid")"

        if [[ "$DB_PID" =~ ^[0-9]+$ ]] &&
           kill -0 "$DB_PID" 2>/dev/null; then
            fail "A process recorded as MariaDB is still running,
but the connection check failed. Check:
$STACK/logs/mariadb-error.log

No second database process was started."
        fi
    fi

    echo "=== Starting MariaDB with local socket access only ==="

    nohup "$SERVER" \
        --no-defaults \
        --basedir="$DB" \
        --datadir="$STACK/data" \
        --socket="$SOCKET" \
        --pid-file="$STACK/run/mariadb.pid" \
        --log-error="$STACK/logs/mariadb-error.log" \
        --skip-networking \
        --innodb-buffer-pool-size=64M \
        --max-connections=10 \
        --performance-schema=OFF \
        > "$STACK/logs/mariadb-launch.log" \
        2>&1 < /dev/null &

    READY=0

    for attempt in $(seq 1 60); do
        if database_ready; then
            READY=1
            break
        fi

        sleep 1
    done

    if [ "$READY" -ne 1 ]; then
        tail -n 60 \
            "$STACK/logs/mariadb-launch.log" \
            "$STACK/logs/mariadb-error.log" \
            2>/dev/null || true

        fail "MariaDB did not become ready."
    fi
fi

# ============================================================
# 8. Create credentials only for a new database
# ============================================================

if [ "$NEW_DATABASE" -eq 1 ]; then
    echo "=== Creating database credentials ==="

    ROOT_PASSWORD="$(
        python -c 'import secrets; print(secrets.token_hex(24))'
    )"

    DEMO_PASSWORD="$(
        python -c 'import secrets; print(secrets.token_hex(16))'
    )"

    # Save recovery credentials privately before applying the SQL.
    cat > "$ROOT_CONFIG" <<EOF
[client]
user=root
password=$ROOT_PASSWORD
socket=$SOCKET
EOF

    cat > "$LOGIN_FILE" <<EOF
phpMyAdmin username: demo
phpMyAdmin password: $DEMO_PASSWORD
Database: demo
EOF

    chmod 600 "$ROOT_CONFIG" "$LOGIN_FILE"

    "$CLIENT" \
        --no-defaults \
        --protocol=socket \
        --socket="$SOCKET" \
        --user=root <<SQL
ALTER USER 'root'@'localhost'
    IDENTIFIED BY '$ROOT_PASSWORD';

CREATE DATABASE demo
    CHARACTER SET utf8mb4;

CREATE USER 'demo'@'localhost'
    IDENTIFIED BY '$DEMO_PASSWORD';

GRANT ALL PRIVILEGES ON demo.*
    TO 'demo'@'localhost';
SQL

else
    echo "=== Preserving existing database credentials ==="
fi

# ============================================================
# 9. Configure phpMyAdmin
# ============================================================

echo "=== Configuring phpMyAdmin ==="

BLOWFISH_SECRET="$(
    python -c 'import secrets; print(secrets.token_hex(16))'
)"

cat > "$STACK/phpmyadmin/config.inc.php" <<'PHP'
<?php

$cfg['blowfish_secret'] = '__BLOWFISH_SECRET__';

$i = 1;

$cfg['Servers'][$i]['auth_type'] = 'cookie';
$cfg['Servers'][$i]['host'] = 'localhost';
$cfg['Servers'][$i]['socket'] =
    getenv('HOME') . '/binder-web/run/mariadb.sock';

$cfg['Servers'][$i]['AllowNoPassword'] = false;

$cfg['TempDir'] =
    getenv('HOME') . '/binder-web/pma-tmp';

$cfg['AllowArbitraryServer'] = false;
PHP

python - "$STACK/phpmyadmin/config.inc.php" "$BLOWFISH_SECRET" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])

path.write_text(
    path.read_text().replace(
        "__BLOWFISH_SECRET__",
        sys.argv[2],
    )
)
PY

# ============================================================
# 10. Stop only the previously recorded web-server process
# ============================================================

if [ -s "$WEB_PID" ]; then
    OLD_PID="$(cat "$WEB_PID")"

    if [[ "$OLD_PID" =~ ^[0-9]+$ ]] &&
       kill -0 "$OLD_PID" 2>/dev/null; then

        OLD_COMMAND="$(ps -p "$OLD_PID" -o args= || true)"
        OLD_OWNER="$(ps -p "$OLD_PID" -o uid= | tr -d ' ' || true)"

        if [ "$OLD_OWNER" != "$(id -u)" ] ||
           [[ "$OLD_COMMAND" != *"$PHP"* ]] ||
           [[ "$OLD_COMMAND" != *"$STACK/Caddyfile"* ]]; then
            fail "The recorded web PID belongs to an unexpected process.
It was not stopped."
        fi

        echo "=== Stopping the previous phpMyAdmin web server ==="

        kill "$OLD_PID"

        for attempt in $(seq 1 30); do
            if ! kill -0 "$OLD_PID" 2>/dev/null; then
                break
            fi
            sleep 0.2
        done

        if kill -0 "$OLD_PID" 2>/dev/null; then
            fail "The previous web server has not stopped yet."
        fi
    fi

    rm -f "$WEB_PID"
fi

# ============================================================
# 11. Check port 7070
# ============================================================

python - "$PORT" <<'PY'
import socket
import sys

port = int(sys.argv[1])

try:
    with socket.socket() as sock:
        sock.setsockopt(
            socket.SOL_SOCKET,
            socket.SO_REUSEADDR,
            1,
        )
        sock.bind(("127.0.0.1", port))

except OSError as exc:
    raise SystemExit(
        f"Port {port} is unavailable: {exc}\n"
        "No unrelated process was stopped."
    )
PY

# ============================================================
# 12. Start phpMyAdmin on port 7070
# ============================================================

echo "=== Starting phpMyAdmin on port $PORT ==="

cat > "$STACK/Caddyfile" <<EOF
{
    admin off
    auto_https off
}

http://127.0.0.1:$PORT {
    root * "$STACK/phpmyadmin"
    php_server
}
EOF

"$PHP" validate \
    --config "$STACK/Caddyfile" \
    --adapter caddyfile

nohup "$PHP" run \
    --config "$STACK/Caddyfile" \
    --adapter caddyfile \
    > "$STACK/logs/phpmyadmin.log" \
    2>&1 < /dev/null &

echo "$!" > "$WEB_PID"

# ============================================================
# 13. Verify the HTTP service
# ============================================================

python - "$PORT" <<'PY'
import sys
import time
import urllib.error
import urllib.request

port = int(sys.argv[1])
url = f"http://127.0.0.1:{port}/"

# The local health check does not need environment HTTP proxies.
opener = urllib.request.build_opener(
    urllib.request.ProxyHandler({})
)

for attempt in range(30):
    try:
        with opener.open(url, timeout=3) as response:
            if response.status == 200:
                print(f"phpMyAdmin is responding on port {port}.")
                break

    except urllib.error.HTTPError as exc:
        raise SystemExit(
            f"phpMyAdmin returned HTTP {exc.code}.\n"
            "Check ~/binder-web/logs/phpmyadmin.log"
        )

    except (urllib.error.URLError, TimeoutError):
        time.sleep(1)

else:
    raise SystemExit(
        "The web server did not become ready.\n"
        "Check ~/binder-web/logs/phpmyadmin.log"
    )
PY

echo
echo "=================================================="
echo "MariaDB + phpMyAdmin setup completed"
echo "=================================================="
echo
echo "Local web address: http://127.0.0.1:$PORT/"
echo "MariaDB: local Unix socket only"
echo

if [ -s "$LOGIN_FILE" ]; then
    cat "$LOGIN_FILE"
else
    echo "Use your existing database login credentials."
fi

echo
echo "Browser access requires Jupyter Server Proxy."

PREFIX="${JUPYTERHUB_SERVICE_PREFIX:-/}"
printf 'Proxy path: %s/proxy/%s/\n' "${PREFIX%/}" "$PORT"

echo
echo "Web log: $STACK/logs/phpmyadmin.log"
echo "Database log: $STACK/logs/mariadb-error.log"
echo
echo "Binder storage is temporary. Back up important data."
