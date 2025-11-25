# Fáze 1: Build (Golang image)
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build statické binárky 'home-api'
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o home-api .

# Fáze 2: Runtime (Alpine linux - malý a bezpečný)
FROM alpine:3.21
WORKDIR /
# Potřebujeme certifikáty pro případná HTTPS volání (i když tady jedeme HTTP)
RUN apk add --no-cache ca-certificates
# Zkopírujeme binárku z builderu
COPY --from=builder /app/home-api /home-api
# Spustíme
CMD ["/home-api"]