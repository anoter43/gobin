package main

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"crypto/rand"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"log"
	"maps"
	"mime"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"
)

func writeJSON(w io.Writer, v any) {
	if err := json.MarshalWrite(w, v, jsontext.WithIndent("  ")); err != nil {
		if rw, ok := w.(http.ResponseWriter); ok {
			httpError(rw, http.StatusInternalServerError)
		}
	}
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, v)
}

func httpError(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

type headerMap map[string]string

func (h headerMap) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	for _, k := range slices.Sorted(maps.Keys(h)) {
		if err := enc.WriteToken(jsontext.String(k)); err != nil {
			return err
		}
		if err := json.MarshalEncode(enc, h[k]); err != nil {
			return err
		}
	}
	return enc.WriteToken(jsontext.EndObject)
}

type getResponse struct {
	Args    map[string]string `json:"args"`
	Headers headerMap         `json:"headers"`
	Origin  string            `json:"origin"`
	URL     string            `json:"url"`
	ID      int               `json:"id,omitzero"`
}

type postResponse struct {
	Args    map[string]string `json:"args"`
	Data    string            `json:"data"`
	Files   map[string]string `json:"files"`
	Form    map[string]string `json:"form"`
	Headers headerMap         `json:"headers"`
	Json    any               `json:"json"`
	Origin  string            `json:"origin"`
	URL     string            `json:"url"`
}

func newGetResponse(r *http.Request) getResponse {
	return getResponse{
		Args:    queryArgs(r),
		Headers: requestHeaders(r),
		Origin:  originIP(r),
		URL:     fullURL(r),
	}
}

func queryArgs(r *http.Request) map[string]string {
	args := make(map[string]string, len(r.URL.Query()))
	for k, v := range r.URL.Query() {
		args[k] = v[0]
	}
	return args
}

func requestHeaders(r *http.Request) headerMap {
	headers := make(headerMap, len(r.Header)+1)
	for k, v := range r.Header {
		headers[k] = strings.Join(v, ", ")
	}
	headers["Host"] = r.Host
	return headers
}

func originIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func handleEcho(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if method == "GET" {
			respondJSON(w, newGetResponse(r))
			return
		}
		data, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		r.Body = io.NopCloser(bytes.NewReader(data))
		resp := postResponse{
			Args:    queryArgs(r),
			Data:    string(data),
			Files:   map[string]string{},
			Form:    map[string]string{},
			Headers: requestHeaders(r),
			Origin:  originIP(r),
			URL:     fullURL(r),
		}
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		switch {
		case mediaType == "application/x-www-form-urlencoded":
			if err := r.ParseForm(); err == nil {
				for k, v := range r.PostForm {
					resp.Form[k] = v[0]
				}
			}
		case strings.HasPrefix(mediaType, "multipart/form-data"):
			if err := r.ParseMultipartForm(10 << 20); err == nil {
				for k, fh := range r.MultipartForm.File {
					f, err := fh[0].Open()
					if err != nil {
						continue
					}
					b, _ := io.ReadAll(f)
					f.Close()
					resp.Files[k] = string(b)
				}
				for k, v := range r.MultipartForm.Value {
					resp.Form[k] = v[0]
				}
			}
		case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
			var v any
			if json.Unmarshal(data, &v) == nil {
				resp.Json = v
			}
		}
		respondJSON(w, resp)
	}
}

func fullURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.RequestURI())
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /ip", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, map[string]string{"origin": originIP(r)})
	})

	mux.HandleFunc("GET /user-agent", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, map[string]string{"user-agent": r.UserAgent()})
	})

	mux.HandleFunc("GET /headers", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, map[string]map[string]string{"headers": requestHeaders(r)})
	})

	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.HandleFunc(method+" /"+strings.ToLower(method), handleEcho(method))
	}

	mux.HandleFunc("GET /status/{code}", func(w http.ResponseWriter, r *http.Request) {
		code, err := strconv.Atoi(r.PathValue("code"))
		if err != nil || code < 100 || code > 599 {
			httpError(w, http.StatusBadRequest)
			return
		}
		w.WriteHeader(code)
	})

	mux.HandleFunc("GET /delay/{seconds}", func(w http.ResponseWriter, r *http.Request) {
		seconds, err := strconv.ParseFloat(r.PathValue("seconds"), 64)
		if err != nil || seconds < 0 || seconds > 10 {
			httpError(w, http.StatusBadRequest)
			return
		}
		time.Sleep(time.Duration(seconds * float64(time.Second)))
		handleEcho("GET").ServeHTTP(w, r)
	})

	mux.HandleFunc("GET /bytes/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("n"))
		if err != nil || n < 0 || n > 100_000 {
			httpError(w, http.StatusBadRequest)
			return
		}
		buf := make([]byte, n)
		if _, err := rand.Read(buf); err != nil {
			httpError(w, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(buf)
	})

	mux.HandleFunc("GET /stream/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("n"))
		if err != nil || n < 0 || n > 1000 {
			httpError(w, http.StatusBadRequest)
			return
		}
		fl, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "application/json")
		for i := range n {
			resp := newGetResponse(r)
			resp.ID = i
			respondJSON(w, resp)
			fmt.Fprint(w, "\n")
			if fl != nil {
				fl.Flush()
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	mux.HandleFunc("GET /uuid", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, map[string]string{"uuid": uuid.New().String()})
	})

	mux.HandleFunc("GET /redirect/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("n"))
		if err != nil || n < 0 || n > 10 {
			httpError(w, http.StatusBadRequest)
			return
		}
		if n == 0 {
			respondJSON(w, newGetResponse(r))
			return
		}
		http.Redirect(w, r, "/redirect/"+strconv.Itoa(n-1), http.StatusFound)
	})

	mux.HandleFunc("GET /html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><html><head><title>gobin</title></head><body><h1>Moby-Dick</h1><p>Call me Ishmael.</p></body></html>")
	})

	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "User-agent: *\nDisallow: /deny\n")
	})

	mux.HandleFunc("GET /json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"slideshow":{"title":"Sample Slide Show","author":"Yours Truly"}}`)
	})

	mux.HandleFunc("GET /gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		writeJSON(gz, newGetResponse(r))
	})

	mux.HandleFunc("GET /bearer", func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]string{"message": "missing bearer token"})
			return
		}
		respondJSON(w, map[string]any{"authenticated": true, "token": token})
	})

	mux.HandleFunc("GET /base64/{value}", func(w http.ResponseWriter, r *http.Request) {
		decoded, err := base64.URLEncoding.DecodeString(r.PathValue("value"))
		if err != nil {
			httpError(w, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(decoded)
	})

	mux.HandleFunc("GET /response-headers", func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.URL.Query() {
			w.Header().Set(k, v[0])
		}
		respondJSON(w, queryArgs(r))
	})

	addr := cmp.Or(os.Getenv("GOBIN_ADDR"), ":8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
