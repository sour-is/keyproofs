NAME=sour.is-ipseity
BUMP?=current
DATE:=$(shell date -u +%FT%TZ)
HASH:=$(shell git rev-pars HEAD 2> /dev/null)
VERSION:=$(shell BUMP=$(BUMP) ./version.sh)


version:
	@echo $(VERSION)

tag:
	git tag -a v$(VERSION) -m "Version: $(VERSION)"
release:
	@make tag BUMP=patch

run:
	go run -v \
           -ldflags "\
              -X main.AppVersion=$(VERSION) \
              -X main.BuildHash=$(HASH) \
              -X main.BuildDate=$(DATE) \
			" \
	   .

build:
	go build -v \
           -ldflags "\
              -X main.AppVersion=$(VERSION) \
              -X main.BuildHash=$(HASH) \
              -X main.BuildDate=$(DATE) \
			" \
	   .

install: build
	install ./keyproofs /usr/local/bin
	install ./sour.is-keyproofs.service /lib/systemd/system