# Estágio de compilação
FROM golang:1.23-alpine AS builder

# Instalar dependências de compilação
RUN apk add --no-cache git

WORKDIR /app

# Copiar arquivos de dependência
COPY go.mod go.sum ./

# Baixar dependências
RUN go mod download

# Copiar código-fonte
COPY . .

# Compilar a aplicação
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Estágio final
FROM alpine:latest  

# Instalar Pandoc
RUN apk add --no-cache pandoc

WORKDIR /root/

# Copiar o binário compilado do estágio de compilação
COPY --from=builder /app/main .

# Criar diretório para uploads
RUN mkdir -p uploads

# Expor a porta 8080
EXPOSE 8080

# Comando para executar a aplicação
CMD ["./main"]