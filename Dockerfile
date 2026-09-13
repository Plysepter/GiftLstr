# --- frontend build ---
FROM node:22-alpine AS web
WORKDIR /src
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
# pnpm's install occasionally SIGSEGVs (exit 139) while spawning its link worker
# threads under constrained/kernel-specific conditions. It is transient and a
# retry completes cleanly, so bound the step to up to 3 attempts.
RUN corepack enable && \
    i=0; \
    until pnpm install --frozen-lockfile; do \
      i=$((i+1)); \
      echo "pnpm install attempt $i failed; retrying in 2s"; \
      if [ "$i" -ge 3 ]; then echo "pnpm install failed after 3 attempts"; exit 1; fi; \
      sleep 2; \
    done
COPY web/ .
RUN pnpm run build

# --- backend build ---
# go1.26's compiler crashes on the generated modernc.org/sqlite source and on
# large go:embed data (flaky "internal compiler error" panics). Also do NOT use
# the floating 1.27 tag: go1.27.1's compiler segfaults/ICEs on some hosts while
# compiling stdlib itself (types2.selector / internal/runtime/atomic). Pin 1.27.0.
FROM golang:1.27.0-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
RUN rm -rf static && mkdir static
COPY --from=web /src/dist ./static
# go1.27's linker also flakes on this project — mostly working quickly, but
# with an occasional "link: invalid symbol ABI: 2" that goes away when the
# exact same invocation is simply run again. Bound it like the pnpm install
# above (transient > deterministic; fail the build only after 3 clean re-runs).
RUN i=0; \
    until CGO_ENABLED=0 go build -o /giftlstr .; do \
      i=$((i+1)); \
      echo "go build attempt $i failed; retrying in 2s"; \
      if [ "$i" -ge 3 ]; then echo "go build failed after 3 attempts"; exit 1; fi; \
      sleep 2; \
    done

# --- runtime ---
FROM alpine:3.21
RUN adduser -D -u 10001 app
VOLUME /data
ENV DATA_DIR=/data
USER app
EXPOSE 8080
COPY --from=server /giftlstr /usr/local/bin/giftlstr
ENTRYPOINT ["/usr/local/bin/giftlstr"]
