FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o log-collector .

FROM alpine:latest
WORKDIR /
# Vytvoříme složku pro logy
RUN mkdir -p /var/log/iot-app
COPY --from=builder /app/log-collector /log-collector
CMD ["/log-collector"]