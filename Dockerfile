FROM node:22-alpine AS web-builder
WORKDIR /build
COPY web/package.json web/package-lock.json* ./
RUN npm install --registry=https://registry.npmmirror.com
COPY web/ ./
RUN npm run build

FROM node:22-alpine AS server-deps
WORKDIR /app
RUN apk add --no-cache python3 make g++
COPY server/package.json server/package-lock.json* ./
RUN npm install --omit=dev --registry=https://registry.npmmirror.com

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
ENV DATA_DIR=/data
COPY --from=server-deps /app/node_modules ./node_modules
COPY server/package.json ./
COPY server/src ./src
COPY --from=web-builder /build/dist ./public
VOLUME /data
EXPOSE 3000
CMD ["node", "src/index.js"]
