FROM golang:1.25-alpine AS builder

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/openclaw-gateway ./cmd/gateway

FROM alpine:3.22

RUN addgroup -S gateway && adduser -S -G gateway gateway

WORKDIR /app

COPY --from=builder /out/openclaw-gateway /usr/local/bin/openclaw-gateway
COPY configs/config.example.json /app/config.json

USER gateway

EXPOSE 8080

ENTRYPOINT ["openclaw-gateway"]
CMD ["-config", "/app/config.json"]
