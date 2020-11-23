NAME=sour.is-keyproofs
BUMP?=current
DATE:=$(shell date -u +%FT%TZ)
HASH:=$(shell git rev-pars HEAD 2> /dev/null)
VERSION:=$(shell BUMP=$(BUMP) ./version.sh)


build: $(NAME)

clean:
	rm -f $(NAME)

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
              -X main.BuildDate=$(DATE)" .

$(NAME):
	go build -v \
           -o $(NAME) \
           -ldflags "\
              -X main.AppVersion=$(VERSION) \
              -X main.BuildHash=$(HASH) \
              -X main.BuildDate=$(DATE)" .

install: $(NAME)
	install ./$(NAME) /usr/local/bin
	install ./$(NAME).service /lib/systemd/system
	systemctl daemon-reload
