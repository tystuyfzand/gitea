// Copyright 2016 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package public

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"log"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"code.gitea.io/gitea/modules/setting"

	"github.com/shurcooL/httpgzip"
	"gopkg.in/macaron.v1"
)

//go:generate go run -mod=vendor main.go

// Options represents the available options to configure the macaron handler.
type Options struct {
	Directory   string
	IndexFile   string
	SkipLogging bool
	// if set to true, will enable caching. Expires header will also be set to
	// expire after the defined time.
	ExpiresAfter time.Duration
	FileSystem   http.FileSystem
	Prefix       string
}

// Custom implements the macaron static handler for serving custom assets.
func Custom(opts *Options) macaron.Handler {
	return opts.staticHandler(path.Join(setting.CustomPath, "public"))
}

// StaticHandler sets up a new middleware for serving static files in the
func StaticHandler(dir string, opts *Options) macaron.Handler {
	return opts.staticHandler(dir)
}

func (opts *Options) staticHandler(dir string) macaron.Handler {
	// Defaults
	if len(opts.IndexFile) == 0 {
		opts.IndexFile = "index.html"
	}
	// Normalize the prefix if provided
	if opts.Prefix != "" {
		// Ensure we have a leading '/'
		if opts.Prefix[0] != '/' {
			opts.Prefix = "/" + opts.Prefix
		}
		// Remove any trailing '/'
		opts.Prefix = strings.TrimRight(opts.Prefix, "/")
	}
	if opts.FileSystem == nil {
		opts.FileSystem = http.Dir(dir)
	}

	return func(ctx *macaron.Context, log *log.Logger) {
		opts.handle(ctx, log, opts)
	}
}

func (opts *Options) handle(ctx *macaron.Context, log *log.Logger, opt *Options) bool {
	if ctx.Req.Method != "GET" && ctx.Req.Method != "HEAD" {
		return false
	}

	file := ctx.Req.URL.Path
	// if we have a prefix, filter requests by stripping the prefix
	if opt.Prefix != "" {
		if !strings.HasPrefix(file, opt.Prefix) {
			return false
		}
		file = file[len(opt.Prefix):]
		if file != "" && file[0] != '/' {
			return false
		}
	}

	f, err := opt.FileSystem.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		log.Printf("[Static] %q exists, but fails to open: %v", file, err)
		return true
	}

	// Try to serve index file
	if fi.IsDir() {
		// Redirect if missing trailing slash.
		if !strings.HasSuffix(ctx.Req.URL.Path, "/") {
			http.Redirect(ctx.Resp, ctx.Req.Request, path.Clean(ctx.Req.URL.Path+"/"), http.StatusFound)
			return true
		}

		f, err = opt.FileSystem.Open(file)
		if err != nil {
			return false // Discard error.
		}
		defer f.Close()

		fi, err = f.Stat()
		if err != nil || fi.IsDir() {
			return true
		}
	}

	if !opt.SkipLogging {
		log.Println("[Static] Serving " + file)
	}

	// Add an Expires header to the static content
	if opt.ExpiresAfter > 0 {
		ctx.Resp.Header().Set("Expires", time.Now().Add(opt.ExpiresAfter).UTC().Format(http.TimeFormat))
		tag := GenerateETag(string(fi.Size()), fi.Name(), fi.ModTime().UTC().Format(http.TimeFormat))
		ctx.Resp.Header().Set("ETag", tag)
		if ctx.Req.Header.Get("If-None-Match") == tag {
			ctx.Resp.WriteHeader(304)
			return false
		}
	}

	if _, ok := f.(httpgzip.NotWorthGzipCompressing); !ok {
		if g, ok := f.(httpgzip.GzipByter); ok {
			ctx.Resp.Header().Set("Content-Encoding", "gzip")
			rd := bytes.NewReader(g.GzipBytes())
			ctype := mime.TypeByExtension(filepath.Ext(fi.Name()))
			if ctype == "" {
				// read a chunk to decide between utf-8 text and binary
				var buf [512]byte
				grd, _ := gzip.NewReader(rd)
				n, _ := io.ReadFull(grd, buf[:])
				ctype = http.DetectContentType(buf[:n])
				_, err := rd.Seek(0, io.SeekStart) // rewind to output whole file
				if err != nil {
					log.Printf("rd.Seek error: %v\n", err)
					return false
				}
			}
			ctx.Resp.Header().Set("Content-Type", ctype)
			http.ServeContent(ctx.Resp, ctx.Req.Request, file, fi.ModTime(), rd)
			return true
		}
	}

	http.ServeContent(ctx.Resp, ctx.Req.Request, file, fi.ModTime(), f)
	return true
}

// GenerateETag generates an ETag based on size, filename and file modification time
func GenerateETag(fileSize, fileName, modTime string) string {
	etag := fileSize + fileName + modTime
	return base64.StdEncoding.EncodeToString([]byte(etag))
}
