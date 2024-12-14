GO := go
KIND ?= pia2xui
BUILD ?= .build
BALLS ?= .balls
TGTS ?= \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64
REL_NAME_PREFIX ?= $(KIND)
REL_NAME_SUFFIX ?=
REL_DESC ?= automatic release from Makefile build
GH_USER ?= raven428
GH_REPO ?= pia2xui

.PHONY: local
local: build balls release

.PHONY: build
build:
	@/usr/bin/env bash -c ' \
		echo ">> building binaries…" ; \
		/usr/bin/env mkdir -vp "$(BUILD)" && \
		for tgt in $(TGTS); do \
			export GOOS=$${tgt/\/*} ; \
			export GOARCH=$${tgt/*\/} ; \
			echo " > target [$$VER-$$GOOS-$$GOARCH]" && \
			go build -o \
				"$(BUILD)/$(KIND)-$${VER}-$${GOOS}-$${GOARCH}" ; \
		done ; \
	'

.PHONY: balls
balls:
	@/usr/bin/env bash -c ' \
		echo ">> building release balls…" ; \
		/usr/bin/env mkdir -vp "$(BALLS)" && \
		for tgt in $(TGTS); do \
			GOOS=$${tgt/\/*} && \
			GOARCH=$${tgt/*\/} && \
			BALL_PFX="../$(BALLS)/$(KIND)-$${VER}-$${GOOS}-$${GOARCH}" && \
			echo " > target [$$VER-$$GOOS-$$GOARCH]" && \
			( \
				cd "$(BUILD)" && \
				/usr/bin/env tar --create \
					--file $${BALL_PFX}.tar \
					$(KIND)-$${VER}-$${GOOS}-$${GOARCH} && \
				/usr/bin/env pixz $${BALL_PFX}.tar && \
				/usr/bin/env mv -f $${BALL_PFX}.tpxz $${BALL_PFX}.txz ; \
			) \
		done ; \
	'

.PHONY: release
release: get-github-release
	@/usr/bin/env bash -c ' \
		echo ">> pushing binaries to GitHub…" ; \
		if [[ -z "$${GITHUB_TOKEN}" ]]; then \
			echo "Undefined or empty GITHUB_TOKEN environment variable, giving up…"; \
			exit 1; \
		fi; \
		echo " > creating release [$${VER}] draft" && \
		/usr/bin/env git tag -fm master "$${VER}" && \
		/usr/bin/env git push --force origin "$${VER}" && \
		/usr/bin/env github-release release \
			--draft \
			-t "$${VER}" \
			-u "$(GH_USER)" \
			-r "$(GH_REPO)" \
			-n "$(REL_NAME_PREFIX) $${VER} $(REL_NAME_SUFFIX)" \
			-d "$(REL_DESC)" && \
		while ! /usr/bin/env github-release info \
			-t "$${VER}" \
			-u "$(GH_USER)" \
			-r "$(GH_REPO)" 2>/dev/null; do \
			echo "  > waiting to release will be ready…" \
			/usr/bin/env sleep 1; \
		done && \
		for tgt in $(TGTS); do \
			GOOS=$${tgt/\/*} && \
			GOARCH=$${tgt/*\/} && \
			echo "  > arch [$${GOARCH}] for OS [$${GOOS}] uploading" && \
			/usr/bin/env github-release upload \
			-t "$${VER}" \
			-u "$(GH_USER)" \
			-r "$(GH_REPO)" \
			-n "$(KIND)-$${VER}-$${GOOS}-$${GOARCH}.txz" \
			-f "$(BALLS)/$(KIND)-$${VER}-$${GOOS}-$${GOARCH}.txz" ; \
		done ; \
	'

.PHONY: get-github-release
get-github-release:
	# command bellow installing previous version for some reason:
	# $(GO) install github.com/github-release/github-release@latest
	@/usr/bin/env bash -c ' \
		if [[ \
			"$$( \
				/usr/bin/env github-release --version 2>&1 | \
					/usr/bin/env awk '\''{print $$2}'\'' \
			)" != '\''v0.10.0'\'' \
		]]; then \
			/usr/bin/env curl -L https://github.com/github-release/github-release/releases/download/v0.10.0/linux-amd64-github-release.bz2 | \
				/usr/bin/env bzip2 -d >"$${GOPATH}/bin/github-release" && \
				/usr/bin/env chmod 755 "$${GOPATH}/bin/github-release" ; \
		fi \
	'

.PHONY: clean
clean:
	@rm -rfv "$(BUILD)" "$(BALLS)"

.PHONY: gh-act
gh-act:
	@/usr/bin/env bash -c ' \
		/usr/bin/env git tag -fm master "$${VER}" && \
		/usr/bin/env git push --force origin "$${VER}" \
	'
