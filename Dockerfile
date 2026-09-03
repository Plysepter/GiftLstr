# --- frontend build ---
FROM node:22-alpine AS web
WORKDIR /src
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web/ .
ARG VITE_CLERK_PUBLISHABLE_KEY
ENV VITE_CLERK_PUBLISHABLE_KEY=$VITE_CLERK_PUBLISHABLE_KEY
RUN pnpm run build

# --- backend build ---
FROM golang:1.26-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
RUN rm -rf static && mkdir static
COPY --from=web /src/dist ./static
RUN CGO_ENABLED=0 go build -o /giftlstr .

# --- runtime ---
FROM alpine:3.21
RUN adduser -D -u 10001 app
VOLUME /data
ENV DATA_DIR=/data
USER app
EXPOSE 8080
COPY --from=server /giftlstr /usr/local/bin/giftlstr
ENTRYPOINT ["/usr/local/bin/giftlstr"]
