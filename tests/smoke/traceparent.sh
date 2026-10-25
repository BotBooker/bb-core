#!/usr/bin/env bash

# Функция для тестирования одной конкретной ручки с учетом метода
# и ожидаемого HTTP-статуса ее ответа.
run_smoke_test() {
    local base_url=$1
    local method=$2
    local endpoint=$3
    local expected_status=$4

    # Собираем полный URL
    local target_url="${base_url}${endpoint}"

    # Генерируем валидный W3C traceparent
    local trace_id=$(openssl rand -hex 16)
    local parent_id=$(openssl rand -hex 8)
    local traceparent="00-${trace_id}-${parent_id}-01"

    echo "--------------------------------------------------"
    echo "🚀 Тестируем ручку: [$method] $endpoint"
    echo "🔗 URL: $target_url"
    echo "🆔 traceparent: $traceparent"
    echo "🎯 Ожидаемый статус: $expected_status"

    # Выполняем запрос с указанием метода (-X) и сохраняем HTTP-код
    local http_status
    http_status=$(curl -s -o /dev/null -w "%{http_code}" \
        -X "$method" \
        -H "traceparent: $traceparent" \
        "$target_url")

    # Проверяем статус-код (он должен совпасть с ожидаемым для ручки)
    if [ "$http_status" -eq "$expected_status" ]; then
        echo "✅ Успех! Статус: $http_status (ожидался $expected_status)"
        return 0
    else
        echo "❌ Ошибка! Статус: $http_status (ожидался $expected_status)"
        return 1
    fi
}

# === НАСТРОЙКИ ТЕСТА ===

# Задаем BASE_URL: берем из окружения, либо используем дефолтное значение
BASE_URL="${BASE_URL:-http://localhost:8080}"

# Список ручек в формате "МЕТОД:РУЧКА:ОЖИДАЕМЫЙ_СТАТУС"
# Ожидаемый статус — HTTP-код, который ручка отдает на такой запрос
# (без тела и без заголовка Authorization):
#   * открытые ручки (/health, /ready и /auth/*) доходят до своих хендлеров,
#     поэтому отвечают своим кодом: /health и /ready — 200, а /auth/register
#     и /auth/login с пустым телом — 400 (тело не разбирается);
#   * защищенные ручки и неизвестные пути перехватывает middleware
#     authenticate до маршрутизации, поэтому без токена они отвечают 401.
# Вы можете комбинировать любые методы (GET, POST, PUT, DELETE)
ENDPOINTS=(
    "GET:/health:200"
    "GET:/ready:200"
    "POST:/health:405"
    "POST:/ready:405"
    "PUT:/health:405"
    "DELETE:/health:405"
    "GET:/users/me:401"
    "GET:/users/me/da51e111-c3b1-4648-a2d6-e39e7963c70d:401"
    "POST:/auth/register:400"
    "POST:/auth/login:400"
    "POST:/auth/logout:401"
    "GET:/nonexistent/?id=1234:401"
    "POST:/nonexistent:401"
    "PUT:/nonexistent:401"
    "DELETE:/nonexistent:401"
)

# =======================

# Переменная-флаг для отслеживания общего статуса всех тестов
FAILED_TESTS=0

echo "🏁 Начало смок-тестирования для $BASE_URL"

# Цикл по всем элементам массива ENDPOINTS
for item in "${ENDPOINTS[@]}"; do
    # Разделяем строку вида "POST:/auth/login:400" на метод, эндпоинт и статус.
    # Первое двоеточие отделяет метод, последнее — ожидаемый статус
    # (в самих путях двоеточий нет).
    method="${item%%:*}"
    rest="${item#*:}"
    endpoint="${rest%:*}"
    expected_status="${rest##*:}"

    if ! run_smoke_test "$BASE_URL" "$method" "$endpoint" "$expected_status"; then
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
done

echo "=================================================="
# Итоговый результат для CI/CD пайплайна
if [ "$FAILED_TESTS" -eq 0 ]; then
    echo "🎉 Все смок-тесты успешно пройдены!"
    exit 0
else
    echo "🚨 Смок-тест провален! Количество упавших ручек: $FAILED_TESTS"
    exit 1
fi
