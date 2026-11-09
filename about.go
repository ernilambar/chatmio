package main

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// aboutEvent tells the frontend to open the About dialog.
const aboutEvent = "app:about"

//go:embed wails.json
var wailsConfig []byte

//go:embed build/appicon.svg
var appIconSVG []byte

// AboutInfo is the content of the About dialog. Values come from wails.json,
// the same file wails build turns into the bundle's Info.plist, so the version
// shown in the app always matches the bundle.
type AboutInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Copyright   string `json:"copyright"`
	Icon        string `json:"icon"`
}

type appDetails struct {
	Name        string
	Version     string
	Description string
	Copyright   string
}

func readAppDetails() appDetails {
	var cfg struct {
		Info struct {
			ProductName    string `json:"productName"`
			ProductVersion string `json:"productVersion"`
			Comments       string `json:"comments"`
			Copyright      string `json:"copyright"`
		} `json:"info"`
	}
	_ = json.Unmarshal(wailsConfig, &cfg)

	details := appDetails{
		Name:        strings.TrimSpace(cfg.Info.ProductName),
		Version:     strings.TrimSpace(cfg.Info.ProductVersion),
		Description: strings.TrimSpace(cfg.Info.Comments),
		Copyright:   strings.TrimSpace(cfg.Info.Copyright),
	}
	if details.Name == "" {
		details.Name = "Chattu"
	}
	if details.Version == "" {
		details.Version = "dev"
	}
	return details
}

// GetAboutInfo returns the About dialog content for the frontend.
func (a *App) GetAboutInfo() AboutInfo {
	details := readAppDetails()
	return AboutInfo{
		Title:       details.Name,
		Version:     details.Version,
		Description: details.Description,
		Copyright:   details.Copyright,
		Icon:        "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(appIconSVG),
	}
}

// openAbout asks the frontend to show the About dialog.
func (a *App) openAbout() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, aboutEvent)
	}
}
