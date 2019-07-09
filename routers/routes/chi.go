// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routes

import (
	"bytes"
	"fmt"
	"net/http"
	"path"
	"text/template"
	"time"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/public"
	"code.gitea.io/gitea/modules/setting"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

type routerLoggerOptions struct {
	req            *http.Request
	Identity       *string
	Start          *time.Time
	ResponseWriter http.ResponseWriter
}

// SignedUserName returns signed user's name via context
// FIXME currently no any data stored on gin.Context but macaron.Context, so this will
// return "" before we remove macaron totally
func SignedUserName(req *http.Request) string {
	if v, ok := req.Context().Value("SignedUserName").(string); ok {
		return v
	}
	return ""
}

func setupAccessLogger(c chi.Router) {
	logger := log.GetLogger("access")

	logTemplate, _ := template.New("log").Parse(setting.AccessLogTemplate)
	c.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, req)
			identity := "-"
			if val := SignedUserName(req); val != "" {
				identity = val
			}
			rw := w

			buf := bytes.NewBuffer([]byte{})
			err := logTemplate.Execute(buf, routerLoggerOptions{
				req:            req,
				Identity:       &identity,
				Start:          &start,
				ResponseWriter: rw,
			})
			if err != nil {
				log.Error("Could not set up macaron access logger: %v", err.Error())
			}

			err = logger.SendLog(log.INFO, "", "", 0, buf.String(), "")
			if err != nil {
				log.Error("Could not set up macaron access logger: %v", err.Error())
			}
		})
	})
}

// RouterHandler is a macaron handler that will log the routing to the default gitea log
func RouterHandler(level log.Level) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()

			_ = log.GetLogger("router").Log(0, level, "Started %s %s for %s", log.ColoredMethod(req.Method), req.RequestURI, req.RemoteAddr)

			next.ServeHTTP(w, req)

			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)

			status := ww.Status()
			_ = log.GetLogger("router").Log(0, level, "Completed %s %s %v %s in %v", log.ColoredMethod(req.Method), req.RequestURI, log.ColoredStatus(status), log.ColoredStatus(status, http.StatusText(status)), log.ColoredTime(time.Since(start)))
		})
	}
}

// Recovery returns a middleware that recovers from any panics and writes a 500 and a log if so.
// Although similar to macaron.Recovery() the main difference is that this error will be created
// with the gitea 500 page.
func Recovery() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					combinedErr := fmt.Sprintf("PANIC: %v\n%s", err, string(log.Stack(2)))
					http.Error(w, combinedErr, 500)
				}
			}()

			next.ServeHTTP(w, req)
		})
	}
}

// NewGin creates a chi Router
func NewChi() chi.Router {
	c := chi.NewRouter()
	if !setting.DisableRouterLog {
		c.Use(RouterHandler(setting.RouterLogLevel))
	}
	c.Use(Recovery())
	if setting.EnableAccessLog {
		setupAccessLogger(c)
	}
	if setting.ProdMode {
		log.Warn("ProdMode ignored")
	}

	c.Use(public.Custom(
		&public.Options{
			SkipLogging:  setting.DisableRouterLog,
			ExpiresAfter: time.Hour * 6,
		},
	))
	c.Use(public.Static(
		&public.Options{
			Directory:    path.Join(setting.StaticRootPath, "public"),
			SkipLogging:  setting.DisableRouterLog,
			ExpiresAfter: time.Hour * 6,
		},
	))
	c.Use(public.StaticHandler(
		setting.AvatarUploadPath,
		&public.Options{
			Prefix:       "avatars",
			SkipLogging:  setting.DisableRouterLog,
			ExpiresAfter: time.Hour * 6,
		},
	))
	c.Use(public.StaticHandler(
		setting.RepositoryAvatarUploadPath,
		&public.Options{
			Prefix:       "repo-avatars",
			SkipLogging:  setting.DisableRouterLog,
			ExpiresAfter: time.Hour * 6,
		},
	))

	return c
}

// RegisterRoutes registers gin routes
func RegisterRoutes(c chi.Router) {
	m := NewMacaron()
	RegisterMacaronRoutes(m)

	c.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			m.ServeHTTP(w, req)
			//next.ServeHTTP(w, req)
		})
	})

	// for health check
	c.Head("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// robots.txt
	if setting.HasRobotsTxt {
		c.Get("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
			http.ServeFile(w, req, path.Join(setting.CustomPath, "robots.txt"))
		})
	}
}
