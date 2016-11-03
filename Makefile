SRC := $(shell find . -type f -name '*.go')
ASSET := cmd/slek/assets
ASSETS := $(shell find $(ASSET)/assets -type f)
CROSSARCH := amd64 386
CROSSOS := darwin linux openbsd netbsd freebsd windows
CROSS := $(foreach os,$(CROSSOS),$(foreach arch,$(CROSSARCH),$(os)/$(arch)))

.PHONY: lint reset cross

dist: $(SRC) $(ASSET)/assets.go
	gox \
		-osarch="$(CROSS)" \
		-output="dist/{{.OS}}.{{.Arch}}" \
		./cmd/slek

	touch dist

lint:
	@- golint ./slk/...
	@- golint ./output/...
	@- golint ./cmd/...

$(ASSET)/assets.go: $(ASSETS)
	go-bindata -pkg assets -o $@ -prefix $(ASSET)/assets $(ASSET)/assets/...

reset:
	-rm -rf dist
	-rm $(ASSET)/assets.go

