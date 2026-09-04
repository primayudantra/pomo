BIN := $(HOME)/.local/bin/pomo

.PHONY: build install test

build:
	go build -o pomo .

install: build
	mkdir -p $(HOME)/.local/bin
	cp pomo $(BIN)
	chmod +x $(BIN)
	@echo "installed to $(BIN)"

test:
	go test ./...
