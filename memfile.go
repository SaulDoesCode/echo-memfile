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
	Compressable = []string{"", ".txt", ".htm", ".html", ".css", ".toml", ".php", ".js", ".json", ".md", ".mdown", ".xml", ".svg", ".go", ".cgi", ".py", ".pl", ".aspx", ".asp"}
	Platform     = runtime.GOOS
	slash        = getCorrectSlash()
	fslash       = []byte("/")[0]
)

// MemFile - in memory file struct
type MemFile struct {
	ContentType    string
	ETag           string
	DefaultContent []byte
	Content        []byte
	ModTime        time.Time
	Gzipped        bool
}

type MemFileInstance struct {
	Dir          string
	CacheControl string
	Cached       map[string]MemFile
	Server       *echo.Echo
	DevMode      bool
}

func New(server *echo.Echo, dir string, devmode bool) MemFileInstance {
	mfi := MemFileInstance{
		Server: server,
		DevMode: devmode,
		Cached: map[string]MemFile{},
		CacheControl: "private, must-revalidate",
	}

	serverDir, direrr := filepath.Abs(dir)
	check(direrr, true)
	mfi.Dir = serverDir

	mfi.Update()

	mfi.Server.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c ctx) error {
			path := c.Request().URL.Path

			if filepath.Ext(path) == "" {
				if path[len(path)-1] != fslash {
					path = path + "/index.html"
				} else {
					path = path + "index.html"
				}
			}

			if memFile, ok := mfi.Cached[path]; ok {
				return ServeMemFile(c.Response().Writer, c.Request(), memFile, mfi.CacheControl)
			}
			return next(c)
		}
	})

	return mfi
}

func (mfi *MemFileInstance) Update() {
	filelist := []string{}

	filepath.Walk(mfi.Dir, func(location string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {

			servePath := ServablePath(mfi.Dir, location)
			filelist = append(filelist, servePath)

			mf, hasFile := mfi.Cached[servePath]
			if hasFile {
				if !info.ModTime().Equal(mf.ModTime) {
					err = check(mfi.CacheFile(location, servePath), false)
				}
				return err

			} else {
				if mfi.DevMode {
					fmt.Println("New file: ", servePath)
				}
				return check(mfi.CacheFile(location, servePath), false)
			}
		}
		return err
	})

	for filename, _ := range mfi.Cached {
		if !stringSliceContains(filelist, filename) {
			delete(mfi.Cached, filename)
			if mfi.DevMode {
				fmt.Println("No longer serving: ", filename)
			}
		}
	}
}

func (mfi *MemFileInstance) UpdateOnInterval(interval time.Duration) *time.Ticker {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			mfi.Update()
		}
	}()
	return ticker
}

func (mfi *MemFileInstance) CacheFile(location string, servePath string) error {

	apath, err := filepath.Abs(location)
	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile(apath)
	if err != nil {
		return err
	}

	memFile, exists := mfi.Cached[servePath]
	if exists {
		if len(data) == len(memFile.DefaultContent) {
			return nil
		}
		if mfi.DevMode {
			fmt.Println("File Changed: ", servePath)
		}
	}

	fi, err := os.Stat(apath)
	if err == nil {
		memFile.ModTime = fi.ModTime()
	}

	memFile.ContentType = http.DetectContentType(data)

	memFile.Gzipped = false

	for _, ext := range Compressable {
		fext := filepath.Ext(location)
		if fext == ext {
			memFile.Gzipped = true
		}
		switch fext {
		case ".css":
			memFile.ContentType = "text/css"
		case ".js":
			memFile.ContentType = "application/javascript"
		}
	}

	memFile.DefaultContent = data
	if memFile.Gzipped {
		gzipedData, err := CompressBytes(data)
		if err != nil {
			return err
		}
		memFile.Content = gzipedData
	}

	memFile.ETag = RandStr(6)
	mfi.Cached[servePath] = memFile

	return nil
}

func ServablePath(dir string, loc string) string {
	loc = strings.Replace(loc, dir, "", 1)
	if loc[:1] != slash {
		loc = slash + loc
	}
	if Platform == "windows" {
		loc = strings.Replace(loc, "\\", "/", -1)
	}
	return loc
}

func (mfi *MemFileInstance) ServeMemFile(route string, filename string) *echo.Route {
	loc := filename
	if loc[:1] != slash {
		loc = slash + loc
	}
	return mfi.Server.GET(route, func(c ctx) error {
		return mfi.Serve(c.Response().Writer, c.Request(), loc)
	})
}

func (mfi *MemFileInstance) Serve(res http.ResponseWriter, req *http.Request, filename string) error {

	memFile, exists := mfi.Cached[filename]
	if !exists {
		return echo.ErrNotFound
	}

	ServeMemFile(res, req, memFile, mfi.CacheControl)
	return nil
}

func (mfi *MemFileInstance) ServeMF(c ctx, memFile MemFile) error {

	headers := c.Response().Header()
	headers.Set("Etag", memFile.ETag)
	headers.Set("Cache-Control", mfi.CacheControl)
	headers.Set("Vary", "Accept-Encoding")

	rHeader := c.Request().Header
	if match := rHeader.Get("If-None-Match"); match != "" {
		if strings.Contains(match, memFile.ETag) {
			return c.NoContent(304)
		}
	}

	if match := rHeader.Get("If-Match"); match != "" {
		if strings.Contains(match, memFile.ETag) {
			return c.NoContent(304)
		}
	}

	if memFile.Gzipped && strings.Contains(rHeader.Get("Accept-Encoding"), "gzip") {
		headers.Set("Content-Encoding", "gzip")
		return c.Blob(200, memFile.ContentType, memFile.Content)
	}
	return c.Blob(200, memFile.ContentType, memFile.DefaultContent)
}

func (mfi *MemFileInstance) ServeFile(c ctx, filename string) error {
	memFile, exists := mfi.Cached[filename]
	if !exists {
		return echo.ErrNotFound
	}
	return mfi.ServeMF(c, memFile)
}

func ServeMemFile(res http.ResponseWriter, req *http.Request, memFile MemFile, CacheControl string) error {
	res.Header().Set("Etag", memFile.ETag)
	res.Header().Set("Cache-Control", CacheControl)
	res.Header().Set("Content-Type", memFile.ContentType)
	res.Header().Set("Vary", "Accept-Encoding")

	if match := req.Header.Get("If-None-Match"); match != "" {
		if strings.Contains(match, memFile.ETag) {
			res.WriteHeader(304)
			return nil
		}
	}

	if match := req.Header.Get("If-Match"); match != "" {
		if strings.Contains(match, memFile.ETag) {
			res.WriteHeader(304)
			return nil
		}
	}

	if memFile.Gzipped && strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		res.Header().Set("Content-Encoding", "gzip")
		res.WriteHeader(200)
		res.Write(memFile.Content)
	} else {
		res.WriteHeader(200)
		res.Write(memFile.DefaultContent)
	}

	return nil
}

func getCorrectSlash() string {
	if Platform == "windows" {
		return "\\"
	}
	return "/"
}

func check(err error, critical bool) error {
	if err != nil {
		if critical {
			panic(err)
		}
		fmt.Println(err)
	}
	return err
}

func stringSliceContains(list []string, str string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
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
