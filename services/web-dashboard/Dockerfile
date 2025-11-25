# ==========================================
# FÁZE 1: BUILDER (Kompilace)
# ==========================================
# Používáme obraz s nainstalovaným Go, abychom mohli zkompilovat zdrojáky.
# "AS builder" nám umožňuje později odkazovat na tento stav.
FROM golang:1.24-alpine AS builder

# Nastavíme pracovní adresář uvnitř kontejneru.
WORKDIR /app

# Nejprve zkopírujeme soubory definující závislosti.
# Děláme to odděleně, aby Docker mohl využít cache, pokud se nezměnily závislosti.
COPY go.mod ./
# (Pokud by existoval go.sum, zkopírovali bychom i ten: COPY go.mod go.sum ./)

# Stáhneme závislosti (v tomto případě jen standard lib, ale pro jistotu).
RUN go mod download

# Teď zkopírujeme zbytek zdrojových kódů (main.go, handler.go...) A TAKÉ složku templates/
COPY . .

# Kompilace aplikace.
# CGO_ENABLED=0: Vypneme závislost na systémových C knihovnách (statická binárka).
# -ldflags="-w -s": Odstraní debug informace -> menší výsledný soubor.
# -o dashboard: Výsledný soubor se bude jmenovat "dashboard".
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o dashboard .

# ==========================================
# FÁZE 2: RUNTIME (Běhové prostředí)
# ==========================================
# Začínáme s čistým štítem. Alpine Linux je super malý (cca 5MB).
FROM alpine:3.21

# Nastavíme pracovní adresář na kořen (root).
WORKDIR /

# Instalace certifikátů.
# I když dashboard komunikuje přes HTTP, je dobrá praxe je mít, 
# kdybychom v budoucnu volali externí HTTPS API.
RUN apk add --no-cache ca-certificates

# --- KRITICKÝ KROK ---
# 1. Zkopírujeme zkompilovanou binárku z fáze 'builder'.
COPY --from=builder /app/dashboard /dashboard

# 2. Zkopírujeme složku s HTML šablonami.
# Bez tohoto by aplikace spadla s chybou "no such file or directory".
# Cílová cesta '/templates' odpovídá tomu, co hledá Go kód (filepath.Join("templates", ...)).
COPY --from=builder /app/templates /templates

# Otevřeme port 3000 (informativní, Docker Compose to stejně přebije).
EXPOSE 3000

# Spustíme aplikaci.
CMD ["/dashboard"]