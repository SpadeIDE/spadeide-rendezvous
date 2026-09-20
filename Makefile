.PHONY: help build rendezvous linux linux-amd linux-arm release test fuzz vuln clean

help:
	@echo "Targets:"
	@echo "  make build / rendezvous   Binaries + linux tarballs → dist/rendezvous/"
	@echo "  make linux-amd            linux/amd64 only"
	@echo "  make linux-arm            linux/arm64 only"
	@echo "  make release              Build + stage github-release/ (VERSION=0.1.0)"
	@echo "  make test                 go test ./..."
	@echo "  make fuzz                 Short fuzz on control messages"
	@echo "  make vuln                 govulncheck"
	@echo "  make clean                Remove dist/ and github-release/"

build rendezvous:
	./scripts/build-rendezvous.sh

linux-amd:
	ARCH=amd64 ./scripts/build-rendezvous.sh

linux-arm:
	ARCH=arm64 ./scripts/build-rendezvous.sh

# VERSION=0.2.0 make release
release:
	./scripts/build-release.sh $(VERSION)

test:
	cd server && go test ./... -count=1

fuzz:
	cd server && go test ./internal/signal/ -fuzz=FuzzParseControlMessage -fuzztime=10s

vuln:
	cd server && go run golang.org/x/vuln/cmd/govulncheck@latest ./...

clean:
	rm -rf dist github-release
