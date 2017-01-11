// This implements a very basic Swift server
// Everything is stored in memory
//
// This comes from the https://github.com/mitchellh/goamz
// and was adapted for Swift
//
package swifttest

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"mock-swift-server/swift"
)

const (
	DEBUG          = true
	TEST_ACCOUNT   = "tera"
	CONTAINER_TYPE = "container"
)

var responseParams = map[string]bool{
	"content-type":        true,
	"content-language":    true,
	"expires":             true,
	"cache-control":       true,
	"content-disposition": true,
	"content-encoding":    true,
}

var metaHeaders = map[string]bool{
	"Content-Type":        true,
	"Content-Encoding":    true,
	"Content-Disposition": true,
	"X-Object-Manifest":   true,
}

var rangeRegexp = regexp.MustCompile("(bytes=)?([0-9]*)-([0-9]*)")

func (s *SwiftServer) serveHTTP(w http.ResponseWriter, req *http.Request) {
	// ignore error from ParseForm as it's usually spurious.
	req.ParseForm()

	s.mu.Lock()
	defer s.mu.Unlock()

	if DEBUG {
		log.Printf("z swifttest %q %s %s ", req.Method, req.URL, req.Body)
	}
	a := &action{
		srv:   s,
		w:     w,
		req:   req,
		reqId: fmt.Sprintf("%09X", s.reqId),
	}
	s.reqId++

	var r resource
	defer func() {
		switch err := recover().(type) {
		case *swiftError:
			w.Header().Set("Content-Type", `text/plain; charset=utf-8`)
			http.Error(w, err.Message, err.statusCode)
		case nil:
		default:
			panic(err)
		}
	}()

	var resp interface{}
	if req.URL.String() == "/auth/v1.0" {
		username := req.Header.Get("X-Auth-User")
		if DEBUG {
			log.Printf("gg username %s", username)
		}
		if username == "" {
			username = req.Header.Get("X-Storage-User")
			if DEBUG {
				log.Printf("hh username %s", username)
			}
		}
		if username == "" {
			r = s.resourceForURL(req.URL)
			key := req.Header.Get("X-Auth-Token")
			signature := req.URL.Query().Get("temp_url_sig")
			expires := req.URL.Query().Get("temp_url_expires")
			if key == "" && signature != "" && expires != "" {
				accountName, _, _, _ := s.parseURL(req.URL)
				secretKey := ""
				if account, ok := s.Accounts[accountName]; ok {
					secretKey = account.meta.Get("X-Account-Meta-Temp-Url-Key")
				}
				//john add for test
				if DEBUG {
					log.Printf("accountName %s signature %s key %s", accountName, signature, key)
				}
				get_hmac := func(method string) string {
					mac := hmac.New(sha1.New, []byte(secretKey))
					body := fmt.Sprintf("%s\n%s\n%s", method, expires, req.URL.Path)
					mac.Write([]byte(body))
					return hex.EncodeToString(mac.Sum(nil))
				}

				if req.Method == "HEAD" {
					if signature != get_hmac("GET") && signature != get_hmac("POST") && signature != get_hmac("PUT") {
						if DEBUG {
							log.Printf("a1 signature %s key %s", signature, key)
						}
						panic(notAuthorized())
					}
				} else if signature != get_hmac(req.Method) {
					if DEBUG {
						log.Printf("b1 signature %s key %s", signature, key)
					}
					panic(notAuthorized())
				}
			} else {
				session, ok := s.Sessions[key[7:]]
				if !ok {
					if DEBUG {
						log.Printf("c1 signature %s key %s sessions %q ok %t", signature, key, session, ok)
					}
					panic(notAuthorized())
				}

				a.user = s.Accounts[session.username]
			}
			if DEBUG {
				log.Printf("request %s ", req.Method)
			}
			switch req.Method {
			case "PUT":
				resp = r.put(a)

			case "GET", "HEAD":
				resp = r.get(a)

			case "DELETE":
				resp = r.delete(a)

			case "POST":
				resp = r.post(a)

			case "COPY":
				resp = r.copy(a)

			default:
				fatalf(400, "MethodNotAllowed", "unknown http request method %q", req.Method)
			}

			content_type := req.Header.Get("Content-Type")
			if resp != nil && req.Method != "HEAD" {
				if strings.HasPrefix(content_type, "application/json") ||
					req.URL.Query().Get("format") == "json" {
					jsonMarshal(w, resp)
				} else {
					switch r := resp.(type) {
					case string:
						w.Write([]byte(r))
					default:
						w.Write(resp.([]byte))
					}
				}
			}

		} else {
			key := req.Header.Get("X-Auth-Key")
			if key == "" {
				key = req.Header.Get("X-Storage-Pass")
			}
			spltedUsername := strings.Split(username, ":")
			if DEBUG {
				log.Printf("y username %s key %s ", spltedUsername[0], key)
			}
			if acct, ok := s.Accounts[spltedUsername[0]]; ok {
				if acct.password == key {
					r := make([]byte, 16)
					_, _ = rand.Read(r)
					id := fmt.Sprintf("%X", r)
					w.Header().Set("X-Storage-Url", s.URL+"/AUTH_"+spltedUsername[0])
					w.Header().Set("X-Auth-Token", "AUTH_tk"+string(id))
					w.Header().Set("X-Storage-Token", "AUTH_tk"+string(id))
					s.Sessions[id] = &session{
						//username: spltedUsername[0],
						//username: username,
						username: spltedUsername[0],
					}
					if DEBUG {
						log.Printf("x username %s key %s token %s ", spltedUsername[0], key, string(id))
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			if DEBUG {
				log.Printf("d username %s key %s ", spltedUsername[0], key)
			}
			panic(notAuthorized())
		}
	}

	if req.URL.String() == "/info" {
		jsonMarshal(w, &swift.SwiftInfo{
			"swift": map[string]interface{}{
				"version": "1.2",
			},
			"tempurl": map[string]interface{}{
				"methods": []string{"GET", "HEAD", "PUT"},
			},
		})
		return
	}

	r = s.resourceForURL(req.URL)

	token := req.Header.Get("X-Auth-Token")
	if token == "" {
		for fakeKey := range s.Sessions {
			token = fakeKey
			break
		}
	}
	if DEBUG {
		log.Printf("w2 s.session %s, token %s", s.Sessions, token)
	}
	signature := req.URL.Query().Get("temp_url_sig")
	expires := req.URL.Query().Get("temp_url_expires")
	if token == "" && signature != "" && expires != "" {
		accountName, _, _, _ := s.parseURL(req.URL)
		secretKey := ""
		if account, ok := s.Accounts[accountName]; ok {
			secretKey = account.meta.Get("X-Account-Meta-Temp-Url-Key")
		}
		//john add for test

		log.Printf("accountName %s signature %s token %s", accountName, signature, token)

		get_hmac := func(method string) string {
			mac := hmac.New(sha1.New, []byte(secretKey))
			body := fmt.Sprintf("%s\n%s\n%s", method, expires, req.URL.Path)
			mac.Write([]byte(body))
			return hex.EncodeToString(mac.Sum(nil))
		}

		if req.Method == "HEAD" {
			if signature != get_hmac("GET") && signature != get_hmac("POST") && signature != get_hmac("PUT") {
				if DEBUG {
					log.Printf("a signature %s token %s", signature, token)
				}
				panic(notAuthorized())
			}
		} else if signature != get_hmac(req.Method) {
			if DEBUG {
				log.Printf("b signature %s token %s", signature, token)
			}
			panic(notAuthorized())
		}
	} else {
		if DEBUG {
			log.Printf("w s.session %s, token %s", s.Sessions, token)
		}
		//log.Printf("www s.session %s, token 7 %s", s.Sessions, token[7:])
		//keykey := strings.Subtoken[7:]
		token = strings.Replace(token, "AUTH_tk", "", 1)
		if DEBUG {
			log.Printf("token %s", token)
		}
		session, ok := s.Sessions[token]
		if !ok {
			panic(notAuthorized())
		}

		a.user = s.Accounts[session.username]
	}

	switch req.Method {
	case "PUT":
		resp = r.put(a)

	case "GET", "HEAD":
		resp = r.get(a)

	case "DELETE":
		resp = r.delete(a)

	case "POST":
		resp = r.post(a)

	case "COPY":
		resp = r.copy(a)

	default:
		fatalf(400, "MethodNotAllowed", "unknown http request method %q", req.Method)
	}

	content_type := req.Header.Get("Content-Type")
	if resp != nil && req.Method != "HEAD" {
		if strings.HasPrefix(content_type, "application/json") ||
			req.URL.Query().Get("format") == "json" {
			jsonMarshal(w, resp)
		} else {
			switch r := resp.(type) {
			case string:
				w.Write([]byte(r))
			default:
				w.Write(resp.([]byte))
			}
		}
	}
}
