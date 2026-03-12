package http

import (
	"embed"
	"html/template"
)

//go:embed templates/*
var templateFS embed.FS

var (
	uploadPageTemplate   = template.Must(template.ParseFS(templateFS, "templates/upload.html"))
	oidcCallbackTemplate = template.Must(template.ParseFS(templateFS, "templates/oidc_callback.html"))
	embeddedTemplateYAML = mustReadEmbeddedFile("templates/default.template.yaml")
)

func mustReadEmbeddedFile(path string) string {
	content, err := templateFS.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(content)
}

type uploadPageData struct {
	AccessTokenStorageKey  string
	EmbeddedTemplateYAMLJS template.JS
}

type callbackPageData struct {
	AccessTokenStorageKey string
	Token                 template.JS
}
