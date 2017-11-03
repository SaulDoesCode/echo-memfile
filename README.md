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

### Caching files from any path

```go
  location := "../secret_files/pwd.txt"
  route := "/secrets/pwd"
  err := memfile.CacheFile(location, route)
  if err != nil {
    // ...
  }

  // GET http://localhost:1323/secrets/pwd -> pwd.txt
```

### Serving Files Manually

```go
  server.GET("/resource/", func(c echo.Context) error {

    if file, ok := memfile.Cached["/resource.json"]; ok {
      return memfile.ServeEcho(c, file)
    }

    return c.JSON(404, map[string]string{
      "err": "out of luck no resource here",
    })
  })

  // or shorthand
  server.GET("/resource/", memfile.ServeMemFile("/resource.json"))
  // NOTE: the resource's string corresponds to files under the static dir
  // "./assets/resource.json" -> "/resource.json"

```


##### public domain, do whatever man
