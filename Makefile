APP_NAME := iconDump
VERSION  := $(shell date +%Y%m%d-%H%M%S)
LDFLAGS  := -s -w -X 'main.buildVersion=$(VERSION)'
SRC      := ./...

DIST := dist

.PHONY: all mac mac-amd64 mac-arm64 mac-universal win linux clean

all: mac win linux

# ===== macOS =====
mac: mac-amd64 mac-arm64

mac-amd64:
	@echo "==> Build macOS amd64"
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(APP_NAME)-darwin-amd64 .
	cd $(DIST) && zip -q -9 $(APP_NAME)-darwin-amd64-$(VERSION).zip $(APP_NAME)-darwin-amd64

mac-arm64:
	@echo "==> Build macOS arm64"
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(APP_NAME)-darwin-arm64 .
	cd $(DIST) && zip -q -9 $(APP_NAME)-darwin-arm64-$(VERSION).zip $(APP_NAME)-darwin-arm64

mac-universal: mac-amd64 mac-arm64
	@echo "==> Merge macOS universal binary"
	lipo -create -output $(DIST)/$(APP_NAME)-darwin-universal $(DIST)/$(APP_NAME)-darwin-amd64 $(DIST)/$(APP_NAME)-darwin-arm64
	cd $(DIST) && zip -q -9 $(APP_NAME)-darwin-universal-$(VERSION).zip $(APP_NAME)-darwin-universal

# ===== Windows =====
win:
	@echo "==> Build Windows amd64"
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(APP_NAME)-windows-amd64.exe .
	cd $(DIST) && zip -q -9 $(APP_NAME)-windows-amd64-$(VERSION).zip $(APP_NAME)-windows-amd64.exe

# ===== Linux =====
linux:
	@echo "==> Build Linux amd64"
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(APP_NAME)-linux-amd64 .
	cd $(DIST) && zip -q -9 $(APP_NAME)-linux-amd64-$(VERSION).zip $(APP_NAME)-linux-amd64

# ===== Clean =====
clean:
	rm -rf $(DIST)
