package main

import (
	"client/config"
	"client/pkg/core"
	rpcHandler "client/pkg/rpc"
	"embed"
	"net"
	"net/rpc"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	// 1. Load Config
	config.Load()

	// 2. Setup Reverse RPC Hook
	core.OnReverseRPC = func(server *rpc.Server, conn net.Conn) {
		server.Register(new(rpcHandler.ClientRPC))
		server.ServeConn(conn)
	}

	// 3. Create App instance
	app := NewApp()

	// 4. Run Wails
	err := wails.Run(&options.App{
		Title:  "fffrp Client",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
