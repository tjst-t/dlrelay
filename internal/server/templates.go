package server

import "embed"

//go:embed templates
var templateFS embed.FS

func loadTemplate(name string) string {
	data, err := templateFS.ReadFile("templates/" + name)
	if err != nil {
		panic("missing template: " + name)
	}
	return string(data)
}
