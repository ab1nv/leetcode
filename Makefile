GO ?= go

.PHONY: run commit

run:
	$(GO) run ./scripts

commit:
	@git add -A
	@$(GO) run ./scripts/commit
