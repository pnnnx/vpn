VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o vpn .

dist:
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/vpn-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/vpn-darwin-amd64 .

test:
	go test ./...

# Проверка сгенерированных конфигов настоящим xray
test-xray: build
	VPN_HOME=$(CURDIR)/.xray-test ./vpn setup
	XRAY_BIN=$(CURDIR)/.xray-test/bin/xray go test -run Xray -v ./...

vet:
	go vet ./...
	GOOS=darwin go vet ./...

.PHONY: build dist test test-xray vet
