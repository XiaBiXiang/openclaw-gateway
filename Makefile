BINARY=bin/openclaw-gateway
CONFIG=configs/config.example.json
IMAGE=openclaw-gateway:local

.PHONY: build run test fmt docker-build compose-up compose-down clean

build:
	mkdir -p bin
	go build -o $(BINARY) ./cmd/gateway

run:
	go run ./cmd/gateway -config $(CONFIG)

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

docker-build:
	docker build -t $(IMAGE) .

compose-up:
	cd deploy && docker compose up --build

compose-down:
	cd deploy && docker compose down

clean:
	rm -rf bin dist release
