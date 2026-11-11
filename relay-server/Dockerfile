# Relay chạy trên Bun. Không có dependency runtime nên không cần bước install.
# Cổng 8789 để không đụng 8787 (zen-gate trên VPS) và 8788 (zen-gate WSL).
FROM oven/bun:1-alpine

WORKDIR /app

ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=8789

COPY --chown=bun:bun package.json tsconfig.json ./
COPY --chown=bun:bun src ./src

USER bun

EXPOSE 8789

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:8789/healthz || exit 1

CMD ["bun", "run", "src/index.ts"]
