# ─── 构建阶段 ────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 先复制依赖清单，利用层缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译（静态链接，不依赖 glibc）
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server ./cmd/api

# ─── 运行阶段 ────────────────────────────────────────────────
FROM alpine:3.21

WORKDIR /app

# 时区数据 + 根证书
RUN apk add --no-cache tzdata ca-certificates && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 只拷贝编译产物和配置目录
COPY --from=builder /app/server .
COPY configs/ ./configs/

# 上传目录（挂载卷时自动覆盖，这里保证目录存在）
RUN mkdir -p uploads

EXPOSE 8080

ENTRYPOINT ["./server"]
