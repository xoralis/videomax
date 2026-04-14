# ─── 构建阶段 ────────────────────────────────────────────────
FROM node:22-alpine AS builder

WORKDIR /app

# 启用 corepack 以使用 pnpm
RUN corepack enable

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .
RUN pnpm build

# ─── 运行阶段 ────────────────────────────────────────────────
FROM nginx:1.27-alpine

# 将构建产物放入 nginx 默认目录
COPY --from=builder /app/dist /usr/share/nginx/html

# nginx 配置：SPA history 路由支持 + 反向代理 /api 到后端
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
