.PHONY: build run test clean docker-build docker-up

BINARY_NAME=qradar-collector

build:
	go build -o $(BINARY_NAME) ./cmd/collector/

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY_NAME) ./cmd/collector/

run: build
	./$(BINARY_NAME)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f state.db*

docker-build:
	docker build -t qradar-collector .

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down
