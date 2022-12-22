SHELL := /bin/bash

.PHONY: machine-deps
machine-deps:
	@./scripts/setup.sh

.PHONY: build
build: machine-deps
	@mage build

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
