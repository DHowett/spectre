all: sync .deploy
.PHONY: sync
public/css/all.min.css: $(foreach n,bootstrap master fonts font-awesome pygments ansi select2 select2-bootstrap,public/css/$(n).css)
	-rm $@
	for i in $^; do yui --type css $$i >> $@; done
public/js/all.min.js: $(foreach n,jquery-2.0.3 bootstrap select2 application,public/js/$(n).js)
	uglifyjs $^ -m > $@
sync: public/css/all.min.css public/js/all.min.js
	rsync -avv ./templates ./public uv:go_pastebin/ --delete
.deploy: paste.linux session.key $(wildcard *.yml)
	gnutar cj $^ | pv -prac -N upload | ssh uv "cd go_pastebin; tar xj && mv paste.linux go_pastebin && restart ghostbin"
	touch .deploy
paste.linux: $(wildcard *.go)
	GOOS=linux GOARCH=amd64 go build -ldflags -w -o paste.linux

