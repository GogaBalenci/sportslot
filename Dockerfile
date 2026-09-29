FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sportslot ./cmd/api

FROM alpine:3.20
# platform-api2.max.ru требует доверия к сертификатам НУЦ Минцифры.
# Если gu-st.ru недоступен при сборке, образ всё равно соберётся (WARN в логе),
# а MAX_BOT_API_BASE_URL можно временно переключить на https://platform-api.max.ru.
RUN apk add --no-cache ca-certificates \
 && cd /usr/local/share/ca-certificates \
 && (wget -q -T 20 -O russian_trusted_root_ca.crt https://gu-st.ru/content/lending/russian_trusted_root_ca_pem.crt \
     && wget -q -T 20 -O russian_trusted_sub_ca.crt https://gu-st.ru/content/lending/russian_trusted_sub_ca_pem.crt \
     || echo "WARN: Russian Trusted CA was not downloaded") \
 && find . -name 'russian_trusted_*' -size -100c -delete \
 && update-ca-certificates
ENV TZ=Europe/Moscow
WORKDIR /app
COPY --from=build /out/sportslot ./sportslot
COPY seed-data ./seed-data
EXPOSE 8080
USER nobody:nobody
ENTRYPOINT ["./sportslot"]
