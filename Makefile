SRC := $(shell find . -type f -name '*.go')
ASSET := cmd/slek/assets
ASSETS := $(shell find $(ASSET)/assets -type f)
CROSSARCH := amd64 386
CROSSOS := darwin linux openbsd netbsd freebsd
CROSS := $(foreach os,$(CROSSOS),$(foreach arch,$(CROSSARCH),dist/$(os).$(arch)))

.PHONY: lint reset cross

dist/slek: $(SRC) $(ASSET)/assets.go
	go build -o $@ ./cmd/slek

lint:
	@- golint ./slk/...
	@- golint ./output/...
	@- golint ./cmd/...

$(ASSET)/assets.go: $(ASSETS)
	go-bindata -pkg assets -o $@ -prefix $(ASSET)/assets $(ASSET)/assets/...

cross: $(CROSS)

$(CROSS): $(SRC) cmd/slek/assets/assets.go
	echo $@
	gox \
		-osarch=$(shell basename $@ | sed 's/\./\//') \
		-output="dist/{{.OS}}.{{.Arch}}" \
		./cmd/slek

reset:
	-rm -rf dist
	-rm $(ASSET)/assets.go

