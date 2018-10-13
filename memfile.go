package memfile

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo"
)

type ctx = echo.Context

var (
	// Compressable - list of compressable file types, append to it if needed
	Compressable = []string{"", ".txt", ".htm", ".html", ".css", ".toml", ".php", ".js", ".json", ".md", ".mdown", ".xml", ".svg", ".go", ".cgi", ".py", ".pl", ".aspx", ".asp"}
	// RandomDictionary - String of Characters used in Etag generation, change in needed
	RandomDictionary = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	platform         = runtime.GOOS
	slash            = getCorrectSlash()
	fslash           = []byte("/")[0]
)

// MemFile - in memory file struct
type MemFile struct {
	ContentType    string
	ETag           string
	PushAssets     []string
	DefaultContent []byte
	Content        []byte
	ModTime        time.Time
	mutex          sync.RWMutex
	PushOptions    *http.PushOptions
	Gzipped        bool
}

type MemFileInstance struct {
	Dir          string
	CacheControl string
	Cached       *sync.Map
	Server       *echo.Echo
	DevMode      bool
}

func New(server *echo.Echo, dir string, devmode bool) MemFileInstance {
	mfi := MemFileInstance{
		Server:       server,
		DevMode:      devmode,
		Cached:       &sync.Map{},
		CacheControl: "private, must-revalidate",
	}

	serverDir, direrr := filepath.Abs(dir)
	check(direrr, true)
	mfi.Dir = serverDir

	mfi.Update()

	mfi.Server.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c ctx) error {
			err := next(c)
			if err == nil {
				return err
			}
			path := c.Request().URL.Path

			if filepath.Ext(path) == "" {
				if path[len(path)-1] != fslash {
					path = path + "/index.html"
				} else {
					path = path + "index.html"
				}
			}

			result, exists := mfi.Cached.Load(path)
			if exists {
				respWriter := c.Response().Writer
				mf := result.(*MemFile)

				if len(mf.PushAssets) > 0 {
					if pusher, ok := respWriter.(http.Pusher); ok {
						pushOptions := &http.PushOptions{
							Header: http.Header{
								"Accept-Encoding": c.Request().Header["Accept-Encoding"],
							},
						}
						for _, pushAsset := range mf.PushAssets {
							err := pusher.Push(pushAsset, pushOptions)
							if mfi.DevMode && err != nil {
								fmt.Println("http2-push fail on:", pushAsset, err)
							}
						}
					}
				}

				return ServeMemFile(respWriter, c.Request(), mf, mfi.CacheControl)
			}

			return err
		}
	})

	return mfi
}

func (mfi *MemFileInstance) Update() {
	filelist := []string{}

	filepath.Walk(mfi.Dir, func(location string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Ext(location) != ".push" {

			servePath := ServablePath(mfi.Dir, location)
			filelist = append(filelist, servePath)

			if result, hasFile := mfi.Cached.Load(servePath); hasFile {
				mf := result.(*MemFile)
				if !info.ModTime().Equal(mf.ModTime) {
					err = check(mfi.CacheFile(location, servePath), false)
				}
				return err
			} else {
				if mfi.DevMode {
					fmt.Println("New file Found: ", servePath, " from ", location)
				}
				return check(mfi.CacheFile(location, servePath), false)
			}
		}
		return err
	})

	mfi.Cached.Range(func(filename interface{}, value interface{}) bool {
		if !stringSliceContains(filelist, filename.(string)) {
			mfi.Cached.Delete(filename)
			if mfi.DevMode {
				fmt.Println("No longer serving: ", filename)
			}
		}
		return false
	})
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

	var memFile *MemFile

	result, exists := mfi.Cached.Load(servePath)
	if exists {
		memFile = result.(*MemFile)
		if bytes.Equal(data, memFile.DefaultContent) {
			return nil
		}
		if mfi.DevMode {
			fmt.Println("File Changed: ", servePath)
		}
	} else {
		memFile = &MemFile{}
		mfi.Cached.Store(servePath, memFile)
	}

	// mutating things causes trouble when loads of stuff is accessing
	// the MemFile so I'm locking it down while a file is being updated
	memFile.mutex.Lock()

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

	fi, err := os.Stat(apath)
	if err == nil {
		memFile.ModTime = fi.ModTime()
	}

	pushAssetsFile, err := ioutil.ReadFile(apath + ".push")
	if err == nil {
		var PushAssets []string
		json.Unmarshal(pushAssetsFile, &PushAssets)
		memFile.PushAssets = PushAssets
	}

	memFile.ETag = RandStr(6)

	memFile.mutex.Unlock()
	return nil
}

func ServablePath(dir string, loc string) string {
	loc = strings.Replace(loc, dir, "", 1)
	if loc[:1] != slash {
		loc = slash + loc
	}
	if platform == "windows" {
		loc = strings.Replace(loc, "\\", "/", -1)
	}
	return loc
}

func (mfi *MemFileInstance) ServeMemFile(route string, filename string) {
	loc := filename
	if loc[:1] != slash {
		loc = slash + loc
	}

	mfi.Server.GET(route, func(c ctx) error {
		return mfi.Serve(c.Response().Writer, c.Request(), loc)
	})
}

func (mfi *MemFileInstance) Serve(res http.ResponseWriter, req *http.Request, servePath string) error {
	result, exists := mfi.Cached.Load(servePath)
	if exists {
		return ServeMemFile(res, req, result.(*MemFile), mfi.CacheControl)
	}
	return echo.ErrNotFound
}

func (mfi *MemFileInstance) SetPushAssets(servePath string, PushAssets []string) bool {
	result, exists := mfi.Cached.Load(servePath)
	if exists {
		mf := result.(*MemFile)
		mf.mutex.Lock()
		mf.PushAssets = PushAssets
		mf.mutex.Unlock()
	}
	return exists
}

func (mfi *MemFileInstance) ServeMF(c ctx, memFile *MemFile) error {
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
	result, exists := mfi.Cached.Load(filename)
	if !exists {
		return echo.ErrNotFound
	}
	return mfi.ServeMF(c, result.(*MemFile))
}

func ServeMemFile(res http.ResponseWriter, req *http.Request, memFile *MemFile, CacheControl string) error {
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
	if platform == "windows" {
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
	bits := make([]byte, size)
	rand.Read(bits)
	for k, v := range bits {
		bits[k] = RandomDictionary[v%byte(len(RandomDictionary))]
	}
	return bits
}

func RandStr(size int) string {
	return string(RandBytes(size))
}
