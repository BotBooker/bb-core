# syntax=docker/dockerfile:1.7
# Multi-stage сборка: статические бинарники без CGO, runtime — scratch.
# Миграции не встроены в бинарники (embed отключён): SQL-файлы копируются в
# образ отдельным слоем и читаются с диска из /app/migrations (MIGRATIONS_DIR).

FROM golang:1.27.3-alpine AS build

# ca-certificates и tzdata нужны в scratch-образе: без них не работают TLS до
# PostgreSQL/OTLP-коллектора и часовые пояса. В runtime попадают только файлы
# этих пакетов, сам Alpine в образ не переходит.
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /src

# Сначала зависимости: слой кэшируется, пока не изменились go.mod/go.sum.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG BUILD_DATE
ARG VERSION=dev

# Все три команды собираются одним вызовом: имена файлов совпадают с именами
# пакетов (api, bootstrap, healthcheck, migrate).
RUN CGO_ENABLED=0 go build -mod=readonly -tags netgo -trimpath \
        -ldflags="-s -w -extldflags \"-static\" -X main.version=${VERSION} -X main.buildDate=${BUILD_DATE}" \
        -o /out/ ./cmd/api ./cmd/bootstrap ./cmd/healthcheck ./cmd/migrate

FROM scratch

# Рабочий каталог — /app: относительно него по умолчанию читается ./migrations.
WORKDIR /app

# Корневые сертификаты для исходящего TLS и база часовых поясов.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/ /app/

# SQL-миграции читаются с диска (embed отключён): путь по умолчанию —
# ./migrations относительно /app, поэтому MIGRATIONS_DIR можно не задавать.
COPY migrations /app/migrations

# В scratch нет /etc/passwd, поэтому пользователь задаётся числовыми UID/GID.
# 55555 — тот же nonroot, что был у distroless и в podSecurityContext Helm-чарта.
USER 55555:55555

EXPOSE 8080 50051

HEALTHCHECK --interval=30s --timeout=3s \
  CMD ["/app/healthcheck"]

CMD ["/app/api"]
