SRC := $(shell find . -type f -name '*.go')
CROSSARCH := amd64 386
CROSSOS := darwin linux #TODO gnotifier: openbsd netbsd freebsd
CROSS := $(foreach os,$(CROSSOS),$(foreach arch,$(CROSSARCH),dist/$(os).$(arch)))

.PHONY: lint reset cross

dist/slek: $(SRC)
	go build -o $@ ./cmd/slek

lint:
	@- golint ./slk/...
	@- golint ./output/...
	@- golint ./cmd/...

cross: $(CROSS)

$(CROSS): $(SRC)
	echo $@
	gox \
		-osarch=$(shell basename $@ | sed 's/\./\//') \
		-output="dist/{{.OS}}.{{.Arch}}" \
		./cmd/slek

reset:
	-rm -rf dist

