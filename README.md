# echo-memfile - read a directory, cache it and serve files straight from memory

### example

```go
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

```

### echo-memfile will serve index.html files in directories

http://localhost:1323/ -> ./assets/index.html   
http://localhost:1323/blog -> ./assets/blog/index.html   

________
##### note
``mfi = MemFileInstance``
from    
``mfi := memfile.New(server *echo.Echo, assetsDir string, devmode bool)``
_______

### Updating files

```go
  // if you want to keep your files updated you can
  // 1 - Update them manually as needed

  // this reads the directory and updates files and etags as needed
  mfi.Update()

  // 2 - Update them regularly on an interval

  // this runs memfile.Update every 5 seconds
  mfi.UpdateOnInterval(time.Second * 5)

  // to stop the interval updater
  ticker := mfi.UpdateOnInterval(time.Second * 5)
  if NeedsToStop {
    ticker.Stop()
  }
```

### Caching files from any path

```go
  location := "../secret_files/pwd.txt"
  route := "/secrets/pwd"
  err := mfi.CacheFile(location, route)
  if err != nil {
    // ...
  }

  // GET http://localhost:1323/secrets/pwd -> pwd.txt
```

### Serving Files Manually

```go
  server.GET("/resource/", func(c echo.Context) error {

    if result, ok := mfi.Cached.Load("/resource.json"); ok {
      return mfi.ServeMF(c, result.(*memfile.MemFile))
    }

    return c.JSON(404, map[string]string{
      "err": "out of luck no resource here",
    })
  })

  // or
  server.GET("/resource/", func(c echo.Context) error {
    return mfi.ServeFile(c, "/resource.json")
  })

  // or shorthand
  mfi.ServeMemFile("/resource", "/resource.json")

  // NOTE: the resource's string corresponds to files under the assets dir
  // "./assets/resource.json" -> "/resource.json"
```

### HTTP2 Push with (myfile.html).push

```
	./assets
	>		index.html
	> 	styles.css
	> 	picture.jpg
	> 	index.html.push
```

``index.html.push`` is essentially just a json array containing serve routes to push with your html file
```json
["/styles.css",  "/picture.jpg"]
```

### HTTP2 Push By MemFileInstance.SetPushAssets
Use this method for best results because manually modifying MemFiles can lead to corruption (in memory, your files are safe)
SetPushAssets uses a mutex lock and unlock internally to avoid concurrent read-write related issues.

```go
	mfi.SetPushAssets("/route.html", []string{"/route.css", "/route.js"})
```

### HTTP2 Push By Modifying MemFile.PushAssets ([]string)

```go
result, exists := mfi.Cached.Load("/subdir/index.html")
if exists {
	result.(*memfile.MemFile).PushAssets = []string{"/js/rilti.min.js", "/css/bulma.min.css"}
}
// note if you're doing it this way do it before the server starts
// because of potential MemFile corruption from concurrent read-write issues
// MFI.SetPushAssets uses mutex locks internally to avoid this
```

### API

##### memfile
* ``.New(server *echo.Echo, dir string, devmode bool) MemFileInstance``
* ``.CompressBytes(data []byte) ([]byte, error)`` gzip a byte slice
* ``.ServeMemFile(res http.ResponseWriter, req *http.Request, memFile MemFile, CacheControl string) error``
* ``.ServablePath(dir string, loc string) string`` cleans a filepath (under dir) for use as a url
* ``.RandBytes(size int) []byte``
* ``.RandStr(size int) string``
* ``.MemFileInstance{}``
* ``.MemFile{}``


##### MemFileInstance
* ``.CacheFile(location string, servePath string) error`` read a file and serve it at a particular route
* ``.Update()`` check files for changes and update accordingly
* ``.UpdateOnInterval(interval time.Duration) *time.Ticker`` Keep updating regularly on a duration
* ``.ServeFile(c echo.Context, filename string) error``
* ``.ServeMF(c echo.Context, memFile *MemFile) error``
* ``.ServeMemFile(route string, filename string)`` shorthand handler
* ``.Serve(res http.ResponseWriter, req *http.Request, filename string) error`` for other middleware etc.
* ``.CacheControl`` the Cache-Control header is ``"private, must-revalidate"`` by default, but you can change it
* ``.DevMode``
* ``.Cached``:``*sync.Map``, this contains all the MemFile's in an Instance [assumed types (key string, value MemFile)]


#### public domain, do whatever man
