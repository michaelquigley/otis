package bok

import (
	"bytes"

	"github.com/michaelquigley/df/dd"
)

type frontmatter struct {
	Title   string
	Tags    []string
	Created string
	Extra   map[string]any `dd:",+extra"`
}

func parseFrontmatter(raw []byte) (*frontmatter, []byte, error) {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return &frontmatter{}, raw, nil
	}
	end := bytes.Index(raw[len("---\n"):], []byte("\n---\n"))
	if end < 0 {
		return &frontmatter{}, raw, nil
	}
	headerStart := len("---\n")
	headerEnd := headerStart + end
	bodyStart := headerEnd + len("\n---\n")
	fm, err := dd.NewYAML[frontmatter](raw[headerStart:headerEnd])
	if err != nil {
		return nil, nil, err
	}
	if len(fm.Extra) == 0 {
		fm.Extra = nil
	}
	return fm, raw[bodyStart:], nil
}
