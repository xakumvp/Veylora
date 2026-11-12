// Veylora — a local desktop app for building and running visual automations.
//
// Wails always builds the module root, so the entry point lives here rather than
// in cmd/. All application logic is in internal/.
package main

import (
	"embed"
	"log"

	"veylora/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is overridden at release build time: -ldflags "-X main.version=v0.1.0".
var version = "0.2.0-dev"

func main() {
	if err := app.Run(assets, version); err != nil {
		log.Fatal(err)
	}
}
