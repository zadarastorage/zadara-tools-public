.PHONY: all linux

all: linux

linux:
	go mod tidy
	GOOS=linux CGO_ENABLED=1 go build -o bin/zadara-metering-linux-amd64 .
