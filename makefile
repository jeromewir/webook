APP_NAME := webook

.PHONY: all build run clean dev install-air

all: build

build:
	go build -o bin/$(APP_NAME) .

run: build
	./bin/$(APP_NAME)

clean:
	rm -rf bin

dev:
	go tool air -build.exclude_dir chrome-data