
FROM golang:1.23-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o evcc-charging-planner main.go
# ==============================================
FROM alpine:latest

RUN apk add --no-cache tzdata
COPY --from=builder /app/evcc-charging-planner /usr/local/bin/evcc-charging-planner
ENTRYPOINT ["evcc-charging-planner"]

