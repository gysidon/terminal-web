# 阶段 1：构建前端 web/dist
FROM node:22-alpine AS web-builder
WORKDIR /build
COPY web/package.json web/package-lock.json* ./
RUN npm install --registry=https://registry.npmmirror.com
COPY web/ ./
RUN npm run build

# 阶段 2：编译 Go 后端（modernc.org/sqlite 为纯 Go，无需 CGO）
FROM golang:1.25-alpine AS go-builder
WORKDIR /src
RUN apk add --no-cache git
COPY server-go/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server-go -trimpath .

# 阶段 3：运行镜像
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
ENV DATA_DIR=/data
ENV STATIC_DIR=/app/public
ENV PORT=3000
ENV HOST=0.0.0.0
COPY --from=go-builder /out/server-go /app/server-go
COPY --from=web-builder /build/dist /app/public
VOLUME /data
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=10s \
  CMD wget -qO- http://127.0.0.1:3000/api/setup/status >/dev/null 2>&1 || exit 1
CMD ["/app/server-go"]
