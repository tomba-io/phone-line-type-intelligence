.PHONY: build test bench lint fuzz clean proto protolint data-fetch data-build audit

GO         ?= go
GOLINT     ?= golangci-lint
FUZZTIME   ?= 30s

# ── Build ──────────────────────────────────────────────
build:
	$(GO) build -o lti ./cmd/lti
	$(GO) build -o ltbuild ./cmd/ltbuild

# ── Proto ─────────────────────────────────────────────
proto:
	protoc --go_out=. --go_opt=paths=source_relative proto/linetype/v1/linetype.proto

protolint:
	protolint lint proto/

# ── Test ───────────────────────────────────────────────
test:
	$(GO) test -race -v ./...

bench:
	$(GO) test -bench=. -benchmem ./...

fuzz:
	$(GO) test -fuzz=FuzzLookup -fuzztime=$(FUZZTIME) .

# ── Lint ───────────────────────────────────────────────
lint:
	$(GO) vet ./...
	$(GOLINT) run ./...
	protolint lint proto/

# ── Data pipeline ─────────────────────────────────────
DATA ?= _build/linetype

data-fetch:
	DEST=$(CURDIR)/$(DATA) bash scripts/get-data.sh

data-build:
	@CO="$(DATA)/CoCodeAssignment_Utilized_AllStates_Public.txt"; \
	CA=$$(find $(DATA) -name 'ca_cocodes.csv' -o -name 'COCodeStatus_ALL.csv' 2>/dev/null | head -1); \
	if [ -n "$$CA" ]; then CO="$$CO,$$CA"; echo "Including Canadian data: $$CA"; fi; \
	BLOCKS=""; \
	if [ -f "$(DATA)/ThousandsBlockAssignment_All_Augmented.txt" ]; then \
		BLOCKS="-blocks $(DATA)/ThousandsBlockAssignment_All_Augmented.txt"; \
	fi; \
	MX=$$(find $(DATA) -name 'pnn_Publico*.csv' 2>/dev/null | head -1); \
	MX_FLAGS=""; \
	if [ -n "$$MX" ]; then \
		echo "Including Mexico data: $$MX"; \
		MX_FLAGS="-mx $$MX -mx-brands data/mx_brands.csv"; \
	fi; \
	$(GO) run ./cmd/ltbuild \
		-co "$$CO" \
		$$BLOCKS \
		-ocn data/ocn.csv \
		$$MX_FLAGS \
		-proto-out data/phone_data.pb \
		-report

# ── Audit ─────────────────────────────────────────────
audit:
	$(GO) run ./cmd/lti audit --verbose

# ── Clean ─────────────────────────────────────────────
clean:
	rm -f lti ltbuild
	rm -rf _build
	$(GO) clean -testcache
