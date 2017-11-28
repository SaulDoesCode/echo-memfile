package memfile

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"github.com/labstack/echo"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type ctx = echo.Context

var (
	Compressable = []string{"", ".txt", ".htm", ".html", ".css", ".php", ".js", ".json", ".md", ".mdown", ".xml", ".svg", ".go", ".cgi", ".py", ".pl", ".aspx", ".asp"}
)

// MemFile - in memory file struct
type MemFile struct {
	ContentType    string
	ETag           string
	DefaultContent []byte
	Content        []byte
	Gzipped        bool
}

var (
	ServerDir        string
	DevMode          bool
	Cached        = map[string]MemFile{}
	memfilesFirstRun = true
	Platform         = runtime.GOOS
	slash            = "/"
	fslash           = []byte("/")[0]
)

func check(err error) error {
	if err != nil {
		fmt.Println(err)
	}
	return err
}

// CompressBytes convert []byte to gziped []byte
func CompressBytes(data []byte) ([]byte, error) {
	var buff bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buff, 9)
	if err != nil {
		return data, err
	}
	_, err = gz.Write(data)
	if err != nil {
		return data, err
	}
	err = gz.Flush()
	if err != nil {
		return data, err
	}

	return buff.Bytes(), gz.Close()
}

func RandBytes(size int) []byte {

	dictionary := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	bits := make([]byte, size)
	rand.Read(bits)
	for k, v := range bits {
		bits[k] = dictionary[v%byte(len(dictionary))]
	}
	return bits
}

func RandStr(size int) string {
	return string(RandBytes(size))
}

func Update() {
	filelist := []string{}
	filepath.Walk(ServerDir, func(location string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {

			servePath := servablePath(location)
			filelist = append(filelist, servePath)

			if !memfilesFirstRun {
				_, hasFile := Cached[servePath]
				if !hasFile && DevMode {
					fmt.Println("New File: ", servePath)
				}
			} else if DevMode {
				fmt.Println("New file: ", servePath)
			}

			if mferr := CacheFile(location, servePath); mferr != nil && DevMode {
				panic(mferr)
			}
		}
		return err
	})

	for mfPath, _ := range Cached {
		shouldDelete := true
		for _, servePath := range filelist {
			if servePath == mfPath {
				shouldDelete = false
			}
		}
		if shouldDelete {
			delete(Cached, mfPath)
			if DevMode {
				fmt.Println("No longer serving: ", mfPath)
			}
		}
	}
}

func UpdateOnInterval(interval time.Duration) *time.Ticker {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			Update()
		}
	}()
	return ticker
}

func Init(server *echo.Echo, dir string, devmode bool) {
	if Platform == "windows" {
		slash = "\\"
	}
	DevMode = devmode
	serverdir, direrr := filepath.Abs(dir)
	if direrr != nil {
		panic(direrr)
	}
	ServerDir = serverdir

	Update()
	memfilesFirstRun = false

	server.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c ctx) error {
			path := c.Request().URL.Path

			if filepath.Ext(path) == "" {
				if path[len(path)-1] != fslash {
					path = path + "/index.html"
				} else {
					path = path + "index.html"
				}
			}

			if memfile, ok := Cached[path]; ok {
				Serve(c.Response().Writer, c.Request(), memfile)
				return nil
			}
			return next(c)
		}
	})
}

func CacheFile(location string, servePath string) error {

	apath, err := filepath.Abs(location)
	if err != nil {
		return err
	}

	var data []byte
	data, err = ioutil.ReadFile(apath)
	if err != nil {
		return err
	}

	memfile, exists := Cached[servePath]
	if exists {
		oldlen := len(memfile.DefaultContent)
		newlen := len(data)
		if newlen == oldlen && string(memfile.DefaultContent) == string(data) {
			return nil
		}
		if DevMode {
			fmt.Println("File Changed: ", servePath)
		}
	}

	memfile.ContentType = http.DetectContentType(data)

	shouldCompress := false

	for _, ext := range Compressable {
		fext := filepath.Ext(location)
		if fext == ext {
			shouldCompress = true
		}
		switch fext {
		case ".css":
			memfile.ContentType = "text/css"
		case ".js":
			memfile.ContentType = "application/javascript"
		}
	}

	memfile.DefaultContent = data
	memfile.Gzipped = shouldCompress
	if shouldCompress {
		gzipedData, err := CompressBytes(data)
		if err != nil {
			return err
		}
		memfile.Content = gzipedData
	}

	memfile.ETag = RandStr(6)
	Cached[servePath] = memfile

	return nil
}

func servablePath(loc string) string {
	loc = strings.Replace(loc, ServerDir, "", 1)
	if loc[:1] != slash {
		loc = slash + loc
	}
	if Platform == "windows" {
		loc = strings.Replace(loc, "\\", "/", -1)
	}
	return loc
}

func ServeMemFile(filename string) func(c ctx) error {
	loc := filename
	if loc[:1] != slash {
		loc = slash + loc
	}
	return func(c ctx) error {
		if memfile, ok := Cached[loc]; ok {
			Serve(c.Response().Writer, c.Request(), memfile)
			return nil
		}
		return echo.ErrNotFound
	}
}

func Serve(res http.ResponseWriter, req *http.Request, memfile MemFile) error {
	res.Header().Set("Etag", memfile.ETag)
	//c.Response().Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
	if DevMode {
		res.Header().Set("Cache-Control", "private, must-revalidate")
	} else {
		res.Header().Set("Cache-Control", "private, max-age=600, must-revalidate")
	}
	res.Header().Set("Content-Type", memfile.ContentType)
	res.Header().Set("Vary", "Accept-Encoding")

	if match := req.Header.Get("If-None-Match"); match != "" {
		if strings.Contains(match, memfile.ETag) {
			res.WriteHeader(304)
			return nil
		}
	}

	if match := req.Header.Get("If-Match"); match != "" {
		if strings.Contains(match, memfile.ETag) {
			res.WriteHeader(304)
			return nil
		}
	}

	if memfile.Gzipped && strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		res.Header().Set("Content-Encoding", "gzip")
		res.WriteHeader(200)
		res.Write(memfile.Content)
	} else {
		res.WriteHeader(200)
		res.Write(memfile.DefaultContent)
	}

	return nil
}

func ServeEcho(c ctx, memfile MemFile) error {
	headers := c.Response().Header()
	headers.Set("Etag", memfile.ETag)
	headers.Set("Cache-Control", "private, max-age=30, must-revalidate")
	headers.Set("Vary", "Accept-Encoding")

	rHeader := c.Request().Header
	if match := rHeader.Get("If-None-Match"); match != "" {
		if strings.Contains(match, memfile.ETag) {
			return c.NoContent(304)
		}
	}

	if match := rHeader.Get("If-Match"); match != "" {
		if strings.Contains(match, memfile.ETag) {
			return c.NoContent(304)
		}
	}

	if memfile.Gzipped && strings.Contains(rHeader.Get("Accept-Encoding"), "gzip") {
		headers.Set("Content-Encoding", "gzip")
		return c.Blob(200, memfile.ContentType, memfile.Content)
	}
	return c.Blob(200, memfile.ContentType, memfile.DefaultContent)
}
