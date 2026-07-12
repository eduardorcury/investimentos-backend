# --platform=$BUILDPLATFORM: o builder roda na arquitetura do runner (x86),
# evitando emulação QEMU no passo pesado (go build).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# TARGETARCH é injetado automaticamente pelo BuildKit a partir de --platform
# do build (ex.: "arm64"). O Go cross-compila nativamente para esse alvo.
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -ldflags="-s -w" -o server .

# Diretório temporário vazio para copiar para a imagem scratch (o parser de PDF
# grava arquivos temporários em /tmp via os.CreateTemp/os.MkdirTemp).
RUN mkdir -p /tmpdir


FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder --chmod=1777 /tmpdir /tmp
COPY --from=builder /app/server /server

EXPOSE 8080

ENTRYPOINT ["/server"]
