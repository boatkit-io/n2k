SHELL := /bin/bash

.PHONY: machine-deps
machine-deps:

.PHONY: build
build: machine-deps
	@mage build

.PHONY: lint
lint:
	go run github.com/getoutreach/lintroller/cmd/lintroller@v1.18.1 -config lintroller.yaml ./...
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.58.0 run ./...

.PHONY: test
test: machine-deps
	@mage -v test

.PHONY: testfast
testfast: machine-deps
	@mage -v testfast

.PHONY: codegen
codegen: machine-deps
	@mage -v codegen

.PHONY: clean
clean: machine-deps
	@mage clean
