BIN := $(HOME)/.local/bin/pomo

.PHONY: build install test desktop-dev desktop-build

build:
	go build -o pomo .

install: build
	mkdir -p $(HOME)/.local/bin
	cp pomo $(BIN)
	chmod +x $(BIN)
	@echo "installed to $(BIN)"

test:
	go test ./...

desktop-dev:
	cd desktop && wails dev

desktop-build:
	cd desktop && wails build
	cp desktop/build/bin/pomo-desktop.app/Contents/MacOS/pomo-desktop desktop/build/bin/pomo-desktop
