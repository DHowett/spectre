all: sync .deploy reload_config
.PHONY: sync reload_config
public/css/master.css: public/css/master.less
	lessc $< > $@
public/css/all.min.css: $(foreach n,bootstrap master fonts fontello pygments ansi select2 select2-bootstrap,public/css/$(n).css)
	-rm $@
	for i in $^; do yui --type css $$i >> $@; done
public/js/all.min.js: $(foreach n,jquery-2.0.3 bootstrap select2 application,public/js/$(n).js)
	uglifyjs $^ -m > $@
sync: $(wildcard *.yml) public/css/all.min.css public/js/all.min.js
	rsync -avv *.yml ./templates ./public uv:ghostbin/ --delete
.deploy: paste.linux
	gnutar cj $^ | pv -prac -N upload | ssh uv "cd ghostbin; tar xj && mv paste.linux ghostbin && restart ghostbin"
	touch .deploy
paste.linux: $(wildcard *.go)
	GOOS=linux GOARCH=amd64 go build -ldflags -w -o paste.linux

reload_config:
	ssh uv "killall -HUP ghostbin"

.PHONY: edit-font get-font
FONTELLO_HOST ?= http://fontello.com
edit-font:
	curl --silent --show-error --fail --output .fontello \
		--form "config=@fontello-config.json" \
		${FONTELLO_HOST}
	open ${FONTELLO_HOST}/`cat .fontello`

get-font:
	@if test ! -e .fontello ; then \
		echo 'Run `make edit-font` first.' >&2 ; \
		exit 128 ; \
		fi
	rm -rf .fontello.src .fontello.zip
	curl --silent --show-error --fail --output .fontello.zip \
		${FONTELLO_HOST}/`cat .fontello`/get
	unzip -j .fontello.zip -d .fontello.src
	cp .fontello.src/config.json fontello-config.json
	sed -e 's/..\/font\//..\/fonts\//g' .fontello.src/fontello.css > public/css/fontello.css
	for i in eot woff ttf svg; do cp .fontello.src/fontello.$$i public/fonts; done
	rm -rf .fontello.src .fontello.zip
