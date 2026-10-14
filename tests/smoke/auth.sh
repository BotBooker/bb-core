#!/usr/bin/env bash

# Смок-тест авторизации через curl: проверяет ручки auth.v1.AuthService,
# которые grpc-gateway отдаёт под префиксом /api/v1/auth/*.
#
# Тест идемпотентен: каждого запуска регистрируется пользователь с уникальным
# email (метка времени + случайное число), поэтому повторный запуск не упирается
# в уже занятый адрес.
#
# Запуск:
#   BASE_URL=http://localhost:8080 tests/smoke/auth.sh
#
# Переменные окружения:
#   BASE_URL — адрес приложения (по умолчанию http://localhost:8080).

# Сервер, на котором проверяются ручки.
BASE_URL="${BASE_URL:-http://localhost:8080}"

# Учётные данные уникального пользователя этого запуска.
STAMP="$(date +%s)-${RANDOM}"
EMAIL="smoke-auth-${STAMP}@example.com"
PASSWORD="smoke-password-123"
NEW_PASSWORD="smoke-password-456"
TOKEN=""

# Файл для тела ответа и счётчики результатов.
BODY_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE"' EXIT
PASSED=0
FAILED=0

# report выводит результат одной проверки и копит статистику.
report() {
    local name="$1" expected="$2" got="$3"
    if [ "$got" = "$expected" ]; then
        PASSED=$((PASSED + 1))
        echo "✅ $name: статус $got (ожидался $expected)"
    else
        FAILED=$((FAILED + 1))
        echo "❌ $name: статус $got (ожидался $expected)"
        echo "    тело ответа: $(cat "$BODY_FILE")"
    fi
}

# request выполняет запрос и возвращает HTTP-код, сохраняя тело в BODY_FILE.
#   request <метод> <путь> [тело] [bearer-токен]
request() {
    local method="$1" path="$2" body="${3:-}" token="${4:-}"
    local args=(-s -o "$BODY_FILE" -w '%{http_code}' -X "$method")

    if [ -n "$token" ]; then
        args+=(-H "Authorization: Bearer $token")
    fi
    if [ -n "$body" ]; then
        args+=(-H 'Content-Type: application/json' -d "$body")
    fi
    curl "${args[@]}" "${BASE_URL}${path}"
}

# check проверяет код ответа ручки.
#   check <имя> <ожидаемый статус> <метод> <путь> [тело] [bearer-токен]
check() {
    local name="$1" expected="$2" method="$3" path="$4" body="${5:-}" token="${6:-}"
    report "$name" "$expected" "$(request "$method" "$path" "$body" "$token")"
}

# extract_token выделяет token из тела последнего ответа: оба ответа — вход и
# регистрация — отдают сессию полем "token".
extract_token() {
    BODY="$(cat "$BODY_FILE")"
    echo "$BODY" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
}

# assert_body проверяет, что тело последнего ответа содержит подстроку.
assert_body() {
    local name="$1" needle="$2"
    if grep -qF "$needle" "$BODY_FILE"; then
        PASSED=$((PASSED + 1))
        echo "✅ $name: тело содержит '$needle'"
    else
        FAILED=$((FAILED + 1))
        echo "❌ $name: тело не содержит '$needle'"
        echo "    тело ответа: $(cat "$BODY_FILE")"
    fi
}

echo "🔐 Смок-тест авторизации: $BASE_URL"
echo "👤 Пользователь запуска: $EMAIL"
echo "--------------------------------------------------"

# --- Открытые и защищённые ручки без токена ---
check "GET /health"                       200 GET  "/health"
check "GET /api/v1/auth/me без токена"    401 GET  "/api/v1/auth/me"
check "POST /api/v1/auth/logout без токена" 401 POST "/api/v1/auth/logout" "{}"

# --- Регистрация ---
check "POST /api/v1/auth/register невалидный email" 400 POST "/api/v1/auth/register" \
    "{\"email\":\"не-email\",\"first_name\":\"Иван\",\"last_name\":\"Иванов\",\"password\":\"$PASSWORD\"}"

check "POST /api/v1/auth/register" 200 POST "/api/v1/auth/register" \
    "{\"email\":\"$EMAIL\",\"first_name\":\"Иван\",\"last_name\":\"Иванов\",\"password\":\"$PASSWORD\"}"
assert_body "ответ регистрации содержит email" "$EMAIL"
TOKEN="$(extract_token)"

check "POST /api/v1/auth/register повторно — конфликт" 409 POST "/api/v1/auth/register" \
    "{\"email\":\"$EMAIL\",\"first_name\":\"Иван\",\"last_name\":\"Иванов\",\"password\":\"$PASSWORD\"}"

# --- Профиль по выданному токену ---
check "GET /api/v1/auth/me с токеном" 200 GET "/api/v1/auth/me" "" "$TOKEN"
assert_body "профиль содержит email" "$EMAIL"

# --- Вход ---
check "POST /api/v1/auth/login неверный пароль" 401 POST "/api/v1/auth/login" \
    "{\"email\":\"$EMAIL\",\"password\":\"definitely-wrong\"}"

check "POST /api/v1/auth/login" 200 POST "/api/v1/auth/login" \
    "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
TOKEN="$(extract_token)"

# --- Смена пароля ---
check "POST /api/v1/auth/password" 200 POST "/api/v1/auth/password" \
    "{\"email\":\"$EMAIL\",\"current_password\":\"$PASSWORD\",\"new_password\":\"$NEW_PASSWORD\"}"
TOKEN="$(extract_token)"

check "POST /api/v1/auth/login старым паролем" 401 POST "/api/v1/auth/login" \
    "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"

check "POST /api/v1/auth/login новым паролем" 200 POST "/api/v1/auth/login" \
    "{\"email\":\"$EMAIL\",\"password\":\"$NEW_PASSWORD\"}"

# --- Выход: токен перестаёт приниматься ---
check "POST /api/v1/auth/logout" 200 POST "/api/v1/auth/logout" "{}" "$TOKEN"
check "GET /api/v1/auth/me после выхода" 401 GET "/api/v1/auth/me" "" "$TOKEN"

# --- Прежние JSON-ручки остались на месте (обратная совместимость) ---
check "POST /auth/login (legacy)" 200 POST "/auth/login" \
    "{\"email\":\"$EMAIL\",\"password\":\"$NEW_PASSWORD\"}"
check "POST /auth/register (legacy) пустое тело" 400 POST "/auth/register"

echo "=================================================="
if [ "$FAILED" -eq 0 ]; then
    echo "🎉 Смок-тест авторизации пройден ($PASSED проверок)"
    exit 0
else
    echo "🚨 Смок-тест авторизации провален: $FAILED из $((PASSED + FAILED)) проверок"
    exit 1
fi
