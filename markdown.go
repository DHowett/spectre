package main

import (
	"bytes"

	"github.com/russross/blackfriday"
)

type MkdHtmlRenderer struct {
	blackfriday.Renderer
}

func (h *MkdHtmlRenderer) BlockCode(out *bytes.Buffer, text []byte, lang string) {
	language := LanguageNamed(lang)
	if language == nil {
		h.Renderer.BlockCode(out, text, lang)
		return
	}
	r := bytes.NewReader(text)
	rendered, _ := FormatStream(r, language)
	out.WriteString(`<div class="code code-` + language.DisplayStyle + `">` + rendered + `</div>`)
}

func NewMkdHtmlRenderer() *MkdHtmlRenderer {
	return &MkdHtmlRenderer{blackfriday.HtmlRenderer(blackfriday.HTML_SAFELINK|
		blackfriday.HTML_SANITIZE_OUTPUT|
		blackfriday.HTML_NOFOLLOW_LINKS, "", "")}
}
