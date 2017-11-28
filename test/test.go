package main

import (
	"time"

	"github.com/SaulDoesCode/echo-memfile"
	"github.com/labstack/echo"
)

func main() {
	server := echo.New() // your echo instance

	assetsDir := "./assets" // directory containing your static assets
	devmode := true         // devmode will mostly log what's happening

	// MemFileInstance: read files and apply the middleware
	mfi := memfile.New(server, assetsDir, devmode)

	// Keep your files updated when you're developing
	if devmode {
		mfi.UpdateOnInterval(time.Second * 1)
	}

	server.Start(":1323")
}
