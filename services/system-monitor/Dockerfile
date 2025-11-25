FROM golang:1.24-alpine AS builder
# gopsutil potřebuje gcc pro některé CGO operace, ale základní metriky obvykle jedou bez CGO.
# Pro jistotu povolíme CGO, pokud by gopsutil potřeboval číst /proc přes C volání (na Linuxu obvykle ne).
# Ale zkusíme "pure Go" verzi nejdřív.
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o monitor .

FROM alpine:latest
WORKDIR /
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/monitor /monitor
CMD ["/monitor"]