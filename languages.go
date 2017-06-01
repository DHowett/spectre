package main

import (
	"context"

	"github.com/DHowett/ghostbin/lib/formatting"
	"github.com/DHowett/ghostbin/model"
)

func FormatPaste(ctx context.Context, p model.Paste) (string, error) {
	reader, _ := p.Reader()
	defer reader.Close()
	return formatting.FormatStream(ctx, reader, formatting.LanguageNamed(p.GetLanguageName()))
}
