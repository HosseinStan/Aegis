package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed testconsole/*
var testConsoleFiles embed.FS

// TestConsole serves a deliberately development-only operator interface. The
// production configuration rejects enabling this route.
func TestConsole() http.Handler {
	root, err := fs.Sub(testConsoleFiles, "testconsole")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/test-console", http.FileServer(http.FS(root)))
}
