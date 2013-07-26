all: sync .deploy reload_config
.PHONY: sync reload_config
public/css/all.min.css: $(foreach n,bootstrap master fonts fontello pygments ansi select2 select2-bootstrap,public/css/$(n).css)
	-rm $@
	for i in $^; do yui --type css $$i >> $@; done
public/js/all.min.js: $(foreach n,jquery-2.0.3 bootstrap select2 application,public/js/$(n).js)
	uglifyjs $^ -m > $@
sync: $(wildcard *.yml) public/css/all.min.css public/js/all.min.js
	rsync -avv *.yml ./templates ./public uv:go_pastebin/ --delete
.deploy: paste.linux session.key
	gnutar cj $^ | pv -prac -N upload | ssh uv "cd go_pastebin; tar xj && mv paste.linux go_pastebin && restart ghostbin"
	touch .deploy
paste.linux: $(wildcard *.go)
	GOOS=linux GOARCH=amd64 go build -ldflags -w -o paste.linux

reload_config:
	ssh uv "killall -HUP go_pastebin"
