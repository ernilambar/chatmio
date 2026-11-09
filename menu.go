package main

import (
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// applicationMenu builds the macOS menu bar. The app menu carries About,
// which opens the in-app dialog instead of the system alert, plus the standard
// Hide and Quit shortcuts.
func applicationMenu(app *App) *menu.Menu {
	name := readAppDetails().Name

	appMenu := menu.NewMenu()
	appMenu.AddText("About "+name, nil, func(*menu.CallbackData) { app.openAbout() })
	appMenu.AddSeparator()
	appMenu.AddText("Hide "+name, keys.CmdOrCtrl("h"), func(*menu.CallbackData) { runtime.Hide(app.ctx) })
	appMenu.AddSeparator()
	appMenu.AddText("Quit "+name, keys.CmdOrCtrl("q"), func(*menu.CallbackData) { runtime.Quit(app.ctx) })

	return menu.NewMenuFromItems(
		menu.SubMenu(name, appMenu),
		menu.EditMenu(),
		menu.WindowMenu(),
	)
}
