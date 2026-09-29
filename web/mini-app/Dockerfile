FROM node:20-alpine AS build
WORKDIR /app
COPY web/mini-app/package.json web/mini-app/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile --ignore-scripts
COPY web/mini-app/ ./
ARG VITE_API_BASE_URL=
ARG VITE_DEV_MAX_USER_ID=
ARG VITE_YANDEX_MAPS_API_KEY=
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_DEV_MAX_USER_ID=$VITE_DEV_MAX_USER_ID \
    VITE_YANDEX_MAPS_API_KEY=$VITE_YANDEX_MAPS_API_KEY
RUN pnpm run build

FROM nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY web/mini-app/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
