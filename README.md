# echo-memfile - read a directory, cache it and serve files straight from memory

### example

```go
  import (
    "time"

    "github.com/SaulDoesCode/echo-memfile"
    "github.com/labstack/echo"
  )


  func main() {
    server := echo.New() // your echo instance

    assetsDir := "./assets" // directory containing your static assets
    devmode := true // devmode will mostly log what's happening

    // read directory and apply middleware
    memfile.Init(server, assetsDir, devmode)

    server.Start(":1323")
  }
```

### echo-memfile will serve index.html files in directories

http://localhost:1323/ -> ./assets/index.html   
http://localhost:1323/blog -> ./assets/blog/index.html   


### updating files

```go
  // if you want to keep your files updated you can
  // 1 - Update them manually as needed

  // this reads the directory and updates files and etags as needed
  memfile.Update()

  // 2 - Update them regularly on an interval

  // this runs memfile.Update every 5 seconds
  memfile.UpdateOnInterval(time.Second * 5)

  // to stop the interval updater
  ticker := memfile.UpdateOnInterval(time.Second * 5)
  if NeedsToStop {
    ticker.Stop()
  }
```

##### public domain, do whatever man
