GO=$(shell which go)
NOW=$(shell date +%s)

BINARY_NAME=lxdepot
MAIN_GO_FILE=cmd/lxdepot/lxdepot.go

build:
	$(GO) build -o $(BINARY_NAME) $(MAIN_GO_FILE)
clean:
	$(GO) clean
	rm -f $(BINARY_NAME)
