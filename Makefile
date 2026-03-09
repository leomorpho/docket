.PHONY: build install test lint

build:
	go build -o docket .

install:
	go install .

test:
	go test ./...

lint:
	go vet ./...
