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

	// read directory and apply middleware
	memfile.Init(server, assetsDir, devmode)

	memfile.UpdateOnInterval(time.Second * 2)

	server.Start(":1323")
}
