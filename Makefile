SRC := $(shell find . -type f -name '*.go')
ASSET := cmd/slek/assets
ASSETS := $(shell find $(ASSET)/assets -type f)
CROSSARCH := amd64 386
CROSSOS := darwin linux openbsd netbsd freebsd windows
CROSS := $(foreach os,$(CROSSOS),$(foreach arch,$(CROSSARCH),$(os)/$(arch)))

VERSION := $(shell git describe)

.PHONY: lint reset clean fakeabout about

# Prepare for commit.
clean: reset fakeabout $(ASSET)/assets.go
	rm $(ASSET)/assets/about

# Create a full build for current os/arch.
build: $(SRC) $(ASSET)/assets.go
	go build -o build ./cmd/slek

# Cross compile for release.
dist: $(SRC) $(ASSET)/assets.go
	gox \
		-osarch="$(CROSS)" \
		-output="dist/{{.OS}}.{{.Arch}}" \
		./cmd/slek

	touch dist

fakeabout:
	echo 'Development build' > "$(ASSET)/assets/about"
	echo 'download release here: https://github.com/frizinak/slek/releases' \
		>> $(ASSET)/assets/about
	echo '===========' >> $(ASSET)/assets/about
	cat assets/about.tpl.txt | \
		sed 's/<tag>/?.?.?/' | \
		sed "/<snail-license>/{"$$'\n'"s/<snail-license>//g"$$'\n'"r assets/snail.license"$$'\n'"}" \
		>> "$(ASSET)/assets/about"

about:
	cat assets/about.tpl.txt | \
		sed 's/<tag>/$(VERSION)/' | \
		sed "/<snail-license>/{"$$'\n'"s/<snail-license>//g"$$'\n'"r assets/snail.license"$$'\n'"}" \
		> "$(ASSET)/assets/about"

$(ASSET)/assets/about:
	$(MAKE) about

lint:
	@- golint ./slk/...
	@- golint ./output/...
	@- golint ./cmd/...

$(ASSET)/assets.go: $(ASSETS) $(ASSET)/assets/about
	go-bindata -nometadata -pkg assets -o $@ -prefix $(ASSET)/assets $(ASSET)/assets/...

reset:
	-rm -rf dist
	-rm $(ASSET)/assets.go
	-rm $(ASSET)/assets/about

