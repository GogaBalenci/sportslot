FROM node:20-alpine AS build
WORKDIR /app
COPY web/mini-app/package.json web/mini-app/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile --ignore-scripts
COPY web/mini-app/ ./
ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
RUN pnpm run build

FROM nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY web/mini-app/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
