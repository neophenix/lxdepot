GO=$(shell which go)

BINARY_NAME=lxdepot
MAIN_GO_FILE=cmd/lxdepot/lxdepot.go

build:
	$(GO) build -o $(BINARY_NAME) $(MAIN_GO_FILE)
clean:
	$(GO) clean
	rm -f $(BINARY_NAME)
install:
	mkdir -p /opt/lxdepot
	mkdir -p /opt/lxdepot/web
	mkdir -p /opt/lxdepot/configs
	mkdir -p /opt/lxdepot/bootstrap
	cp lxdepot /opt/lxdepot/
	cp configs/sample.yaml /opt/lxdepot/configs/
	rsync -aqc web/ /opt/lxdepot/web/
