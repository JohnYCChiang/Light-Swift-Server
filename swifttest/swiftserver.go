package swifttest

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"light-swift-server/swift"
)

// The SwiftServer type
type SwiftServer struct {
	t        *testing.T
	reqId    int
	mu       sync.Mutex
	Listener net.Listener
	AuthURL  string
	URL      string
	Accounts map[string]*account
	Sessions map[string]*session
}

type swiftError struct {
	statusCode int
	Code       string
	Message    string
}

type action struct {
	srv   *SwiftServer
	w     http.ResponseWriter
	req   *http.Request
	reqId string
	user  *account
}

type session struct {
	username string
}

type Metadata struct {
	Meta http.Header // metadata to return with requests.
}

type account struct {
	swift.Account
	Metadata
	password   string
	Containers map[string]*Container
}

var pathRegexp = regexp.MustCompile("/v1/AUTH_([a-zA-Z0-9]+)(/([^/]+)(/(.*))?)?")

func (srv *SwiftServer) parseURL(u *url.URL) (account string, container string, object string, err error) {
	m := pathRegexp.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", "", fmt.Errorf("Couldn't parse the specified URI")
	}
	account = m[1]
	container = m[3]
	object = m[5]
	return
}

// resourceForURL returns a resource object for the given URL.
func (srv *SwiftServer) resourceForURL(u *url.URL) (r resource) {
	accountName, containerName, objectName, err := srv.parseURL(u)

	if err != nil {
		fatalf(404, "InvalidURI", err.Error())
	}

	account, ok := srv.Accounts[accountName]
	if !ok {
		fatalf(404, "NoSuchAccount", "The specified account does not exist")
	}

	if containerName == "" {
		return rootResource{}
	}
	b := containerResource{
		name:      containerName,
		container: account.Containers[containerName],
	}

	if objectName == "" {
		return b
	}

	if b.container == nil {
		fatalf(404, "NoSuchContainer", "The specified container does not exist")
	}

	objr := objectResource{
		name:      objectName,
		version:   u.Query().Get("versionId"),
		container: b.container,
	}

	if obj := objr.container.Objects[objr.name]; obj != nil {
		objr.object = obj
	}
	return objr
}

func NewSwiftServer() (*SwiftServer, error) {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %v", listener.Addr(), err)
	}

	// Get working ip by using the local ip
	// that connect to www.google.com:80
	// But if there are not pubic ip ?
	conn, err := net.Dial("udp", "www.google.com:80")

	if err != nil {
		log.Fatal("Dial Failed")
	}
	defer conn.Close()

	ipWithPort := conn.LocalAddr().String()
	listenIP := strings.Split(ipWithPort, ":")[0]

	server := &SwiftServer{
		Listener: listener,
		AuthURL:  "http://" + listenIP + ":8080/auth/v1.0",
		URL:      "http://" + listenIP + ":8080/auth/v1",
		Accounts: make(map[string]*account),
		Sessions: make(map[string]*session),
	}

	server.Accounts[TEST_ACCOUNT] = &account{
		password: TEST_ACCOUNT,
		Metadata: Metadata{
			Meta: make(http.Header),
		},
		Containers: make(map[string]*Container),
	}

	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		server.serveHTTP(w, req)
	}))
	//john add for test
	if DEBUG {
		log.Printf("AuthURL %q URL %q", server.AuthURL, server.URL)
	}

	return server, nil
}

// Close ...
func (s *SwiftServer) Close() {
	s.Listener.Close()
}

func (m Metadata) setMetadata(a *action, resource string) {
	for key, values := range a.req.Header {
		key = http.CanonicalHeaderKey(key)
		if metaHeaders[key] || strings.HasPrefix(key, "X-"+strings.Title(resource)+"-Meta-") {
			if values[0] != "" || resource == "object" {
				m.Meta[key] = values
			} else {
				m.Meta.Del(key)
			}
		}
	}
}

func (m Metadata) getMetadata(a *action) {
	h := a.w.Header()
	for name, d := range m.Meta {
		h[name] = d
	}
}
