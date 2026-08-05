.PHONY: build test bench lint fuzz clean data-fetch data-build audit

GO         ?= go
GOLINT     ?= golangci-lint
FUZZTIME   ?= 30s

# ── Build ──────────────────────────────────────────────
build:
	$(GO) build -o lti ./cmd/lti
	$(GO) build -o ltbuild ./cmd/ltbuild

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

# ── Data pipeline ─────────────────────────────────────
DATA ?= _build/linetype

data-fetch:
	DEST=$(CURDIR)/$(DATA) bash scripts/get-data.sh

data-build:
	@# US/CA tables — CO codes + thousands blocks
	@CO="$(DATA)/CoCodeAssignment_Utilized_AllStates_Public.txt"; \
	CA=$$(find $(DATA) -name 'ca_cocodes.csv' -o -name 'COCodeStatus_ALL.csv' 2>/dev/null | head -1); \
	if [ -n "$$CA" ]; then CO="$$CO,$$CA"; echo "Including Canadian data: $$CA"; fi; \
	BLOCKS=""; \
	if [ -f "$(DATA)/ThousandsBlockAssignment_All_Augmented.txt" ]; then \
		BLOCKS="-blocks $(DATA)/ThousandsBlockAssignment_All_Augmented.txt"; \
	fi; \
	$(GO) run ./cmd/ltbuild \
		-co "$$CO" \
		$$BLOCKS \
		-ocn data/ocn.csv \
		-out data/linetype.bin \
		-carrier-out data/carrier.bin \
		-carriers-out data/carriers.csv \
		-region-out data/region.bin \
		-regions-out data/regions.csv \
		-report
	@# Mexico table (skip if IFT download failed)
	@MX=$$(find $(DATA) -name 'pnn_Publico*.csv' 2>/dev/null | head -1); \
	if [ -n "$$MX" ]; then \
		echo "Building Mexico tables from $$MX"; \
		$(GO) run ./cmd/ltbuild \
			-mx "$$MX" \
			-mx-out data/mx.bin \
			-mx-carriers-out data/mx_carriers.csv \
			-mx-brands data/mx_brands.csv; \
	else \
		echo "No Mexico PNN file found, skipping MX build"; \
	fi

# ── Audit ─────────────────────────────────────────────
audit:
	$(GO) run ./cmd/lti audit --verbose

# ── Clean ─────────────────────────────────────────────
clean:
	rm -f lti ltbuild
	rm -rf _build
	$(GO) clean -testcache
