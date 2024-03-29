NAME=sour.is-keyproofs
BUMP?=current
DATE:=$(shell date -u +%FT%TZ)
HASH:=$(shell git rev-parse HEAD 2> /dev/null)
VERSION:=$(shell BUMP=$(BUMP) ./version.sh)
-include local.mk
DISABLE_VCARD=true

build: $(NAME)

clean:
	rm -f $(NAME)

version:
	@echo $(VERSION)

tag:
	git tag -a v$(VERSION) -m "Version: $(VERSION)"
	git push --follow-tags

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
	systemctl restart $(NAME)
