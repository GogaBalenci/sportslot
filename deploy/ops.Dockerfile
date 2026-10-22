# Мониторинг и бэкапы: Go-утилита поверх образа PostgreSQL, где уже есть pg_dump той же версии, что и база.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sportslot-ops ./cmd/ops

FROM postgres:16-alpine
# Сертификаты НУЦ Минцифры нужны для platform-api2.max.ru (как в образе API).
RUN apk add --no-cache ca-certificates tzdata \
 && cd /usr/local/share/ca-certificates \
 && (wget -q -T 20 -O russian_trusted_root_ca.crt https://gu-st.ru/content/lending/russian_trusted_root_ca_pem.crt \
     && wget -q -T 20 -O russian_trusted_sub_ca.crt https://gu-st.ru/content/lending/russian_trusted_sub_ca_pem.crt \
     || echo "WARN: Russian Trusted CA was not downloaded") \
 && find . -name 'russian_trusted_*' -size -100c -delete \
 && update-ca-certificates
ENV TZ=Europe/Moscow
COPY --from=build /out/sportslot-ops /usr/local/bin/sportslot-ops
ENTRYPOINT ["sportslot-ops"]
