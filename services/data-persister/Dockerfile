##
## Stage 1: Build
##
FROM golang:1.24-alpine AS builder

# Instalace certifikátů (pro HTTPS) a gitu (pro stahování závislostí)
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

# Vytvoření neprivilegovaného uživatele 'appuser'
# Toto je best-practice: v kontejneru nikdy neběžíme jako root, pokud nemusíme.
RUN adduser -D -g '' appuser

WORKDIR /app

# Nejprve zkopírujeme definice závislostí
# Docker layer caching zajistí, že 'go mod download' se spustí znovu jen když změníš go.mod
COPY go.mod go.sum ./
RUN go mod download

# Zkopírujeme zbytek zdrojových kódů
COPY . .

# Kompilace binárky
# CGO_ENABLED=0: Vypne závislost na systémových C knihovnách (libc), vytvoří statickou binárku.
# -ldflags="-w -s": Odstraní debug informace a symbol tables -> menší velikost binárky.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o persister .

##
## Stage 2: Runtime (Production Image)
##
FROM scratch

# Zkopírujeme certifikáty z builderu (nutné pro HTTPS volání nebo secure MQTT)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Zkopírujeme uživatele z builderu
COPY --from=builder /etc/passwd /etc/passwd

# Zkopírujeme zkompilovanou binárku
COPY --from=builder /app/persister /persister

# Přepneme na bezpečného uživatele
USER appuser

# Definujeme vstupní bod
ENTRYPOINT ["/persister"]
