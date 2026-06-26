# syntax=docker/dockerfile:1

# Stage 1: Build frontend
FROM node:22-alpine AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.23-alpine AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o cost-board .

# Stage 3: Minimal runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /build/cost-board .
RUN mkdir -p /data
ENV COST_BOARD_DATA_DIR=/data
EXPOSE 8083
ENTRYPOINT ["./cost-board"]
CMD ["--port", "8083"]
