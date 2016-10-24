bin := slek
os := $(shell uname | tr [:upper:] [:lower:])
$(os)_gox := disabled
src := $(shell find . -type f -name '*.go')

.PHONY: build clean run cross

build: dist/$(bin)_$(os)

cross: dist/$(bin)_darwin dist/$(bin)_linux

$(linux_gox)dist/$(bin)_linux: $(src) | dist
	gox -osarch="linux/amd64" -output="dist/droplet_linux" ./cmd/slek

$(darwin_gox)dist/$(bin)_darwin: $(src) | dist
	gox -osarch="darwin/amd64" -output="dist/$(bin)_darwin" ./cmd/slek

dist/$(bin)_$(os): $(src) | dist
		go build -o "dist/$(bin)_$(os)" ./cmd/slek 

dist:
	mkdir dist

clean:
	rm -rf dist

run:
	go run cmd/slek/*.go

