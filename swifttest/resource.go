package swifttest

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A resource encapsulates the subject of an HTTP request.
// The resource referred to may or may not exist
// when the request is made.
type resource interface {
	put(a *action) interface{}
	get(a *action) interface{}
	post(a *action) interface{}
	delete(a *action) interface{}
	copy(a *action) interface{}
}

type objectResource struct {
	name      string
	version   string
	container *container // always non-nil.
	object    *object    // may be nil.
}

type containerResource struct {
	name      string
	container *container // non-nil if the container already exists.
}

// GET on a container lists the objects in the container.
func (r containerResource) get(a *action) interface{} {
	if r.container == nil {
		fatalf(404, "NoSuchContainer", "The specified container does not exist")
	}

	delimiter := a.req.Form.Get("delimiter")
	marker := a.req.Form.Get("marker")
	prefix := a.req.Form.Get("prefix")
	format := a.req.URL.Query().Get("format")
	parent := a.req.Form.Get("path")

	a.w.Header().Set("X-Container-Bytes-Used", strconv.Itoa(r.container.bytes))
	a.w.Header().Set("X-Container-Object-Count", strconv.Itoa(len(r.container.objects)))
	r.container.getMetadata(a)

	if a.req.Method == "HEAD" {
		return nil
	}

	objects := r.container.list(delimiter, marker, prefix, parent)

	if format == "json" {
		a.w.Header().Set("Content-Type", "application/json")
		var resp []interface{}
		for _, item := range objects {
			if obj, ok := item.(*object); ok {
				resp = append(resp, obj.Key())
			} else {
				resp = append(resp, item)
			}
		}
		return resp
	} else {
		for _, item := range objects {
			if obj, ok := item.(*object); ok {
				a.w.Write([]byte(obj.name + "\n"))
			} else if subdir, ok := item.(Subdir); ok {
				a.w.Write([]byte(subdir.Subdir + "\n"))
			}
		}
		return nil
	}
}

func (r containerResource) delete(a *action) interface{} {
	b := r.container
	if b == nil {
		fatalf(404, "NoSuchContainer", "The specified container does not exist")
	}
	if len(b.objects) > 0 {
		fatalf(409, "Conflict", "The container you tried to delete is not empty")
	}
	delete(a.user.Containers, b.name)
	a.user.Account.Containers--
	return nil
}

func (r containerResource) put(a *action) interface{} {
	if a.req.URL.Query().Get("extract-archive") != "" {
		fatalf(403, "Operation forbidden", "Bulk upload is not supported")
	}

	if r.container == nil {
		if !validContainerName(r.name) {
			fatalf(400, "InvalidContainerName", "The specified container is not valid")
		}
		r.container = &container{
			name:    r.name,
			objects: make(map[string]*object),
			metadata: metadata{
				meta: make(http.Header),
			},
		}
		r.container.setMetadata(a, CONTAINER_TYPE)
		a.user.Containers[r.name] = r.container
		a.user.Account.Containers++
	}

	return nil
}

func (r containerResource) post(a *action) interface{} {
	// When input container is null
	if r.container == nil {
		// Create container
		if !validContainerName(r.name) {
			fatalf(400, "InvalidContainerName", "The specified container is not valid")
		}
		r.container = &container{
			name:    r.name,
			objects: make(map[string]*object),
			metadata: metadata{
				meta: make(http.Header),
			},
		}
		r.container.setMetadata(a, CONTAINER_TYPE)
		a.user.Containers[r.name] = r.container
		a.user.Account.Containers++
		// OLD CODE :: fatalf(400, "Method", "The resource could not be found.")
	} else {
		r.container.setMetadata(a, CONTAINER_TYPE)
		a.w.WriteHeader(201)
		jsonMarshal(a.w, Folder{
			Count: len(r.container.objects),
			Bytes: r.container.bytes,
			Name:  r.container.name,
		})
	}
	return nil
}

func (containerResource) copy(a *action) interface{} { return notAllowed() }

// GET on an object gets the contents of the object.
func (objr objectResource) get(a *action) interface{} {
	var (
		etag   []byte
		reader io.Reader
		start  int
		end    int = -1
	)
	obj := objr.object
	if obj == nil {
		fatalf(404, "Not Found", "The resource could not be found.")
	}

	h := a.w.Header()
	// add metadata
	obj.getMetadata(a)

	if r := a.req.Header.Get("Range"); r != "" {
		m := rangeRegexp.FindStringSubmatch(r)
		if m[2] != "" {
			start, _ = strconv.Atoi(m[2])
		}
		if m[3] != "" {
			end, _ = strconv.Atoi(m[3])
		}
	}

	max := func(a int, b int) int {
		if a > b {
			return a
		}
		return b
	}

	if manifest, ok := obj.meta["X-Object-Manifest"]; ok {
		var segments []io.Reader
		components := strings.SplitN(manifest[0], "/", 2)
		segContainer := a.user.Containers[components[0]]
		prefix := components[1]
		resp := segContainer.list("", "", prefix, "")
		sum := md5.New()
		cursor := 0
		size := 0
		for _, item := range resp {
			if obj, ok := item.(*object); ok {
				length := len(obj.data)
				size += length
				sum.Write([]byte(hex.EncodeToString(obj.checksum)))
				if start >= cursor+length {
					continue
				}
				segments = append(segments, bytes.NewReader(obj.data[max(0, start-cursor):]))
				cursor += length
			}
		}
		etag = sum.Sum(nil)
		if end == -1 {
			end = size
		}
		reader = io.LimitReader(io.MultiReader(segments...), int64(end-start))
	} else {
		if end == -1 {
			end = len(obj.data)
		}
		etag = obj.checksum
		reader = bytes.NewReader(obj.data[start:end])
	}

	h.Set("Content-Length", fmt.Sprint(end-start))
	h.Set("ETag", hex.EncodeToString(etag))
	h.Set("Last-Modified", obj.mtime.Format(http.TimeFormat))

	if a.req.Method == "HEAD" {
		return nil
	}

	// TODO avoid holding the lock when writing data.
	_, err := io.Copy(a.w, reader)
	if err != nil {
		// we can't do much except just log the fact.
		log.Printf("error writing data: %v", err)
	}
	return nil
}

// PUT on an object creates the object.
func (objr objectResource) put(a *action) interface{} {
	var expectHash []byte
	if c := a.req.Header.Get("ETag"); c != "" {
		var err error
		expectHash, err = hex.DecodeString(c)
		if err != nil || len(expectHash) != md5.Size {
			fatalf(400, "InvalidDigest", "The ETag you specified was invalid")
		}
	}
	sum := md5.New()
	// TODO avoid holding lock while reading data.
	data, err := ioutil.ReadAll(io.TeeReader(a.req.Body, sum))
	if err != nil {
		fatalf(400, "TODO", "read error")
	}
	gotHash := sum.Sum(nil)
	if expectHash != nil && bytes.Compare(gotHash, expectHash) != 0 {
		fatalf(422, "Bad ETag", "The ETag you specified did not match what we received")
	}
	if a.req.ContentLength >= 0 && int64(len(data)) != a.req.ContentLength {
		fatalf(400, "IncompleteBody", "You did not provide the number of bytes specified by the Content-Length HTTP header")
	}

	// TODO is this correct, or should we erase all previous metadata?
	obj := objr.object
	if obj == nil {
		obj = &object{
			name: objr.name,
			metadata: metadata{
				meta: make(http.Header),
			},
		}
		a.user.Objects++
	} else {
		objr.container.bytes -= len(obj.data)
		a.user.BytesUsed -= int64(len(obj.data))
	}

	var content_type string
	if content_type = a.req.Header.Get("Content-Type"); content_type == "" {
		content_type = mime.TypeByExtension(obj.name)
		if content_type == "" {
			content_type = "application/octet-stream"
		}
	}

	// PUT request has been successful - save data and metadata
	obj.setMetadata(a, "object")
	obj.content_type = content_type
	obj.data = data
	obj.checksum = gotHash
	obj.mtime = time.Now().UTC()
	objr.container.objects[objr.name] = obj
	objr.container.bytes += len(data)
	a.user.BytesUsed += int64(len(data))

	h := a.w.Header()
	h.Set("ETag", hex.EncodeToString(obj.checksum))

	return nil
}

func (objr objectResource) delete(a *action) interface{} {
	if objr.object == nil {
		fatalf(404, "NoSuchKey", "The specified key does not exist.")
	}

	objr.container.bytes -= len(objr.object.data)
	a.user.BytesUsed -= int64(len(objr.object.data))
	delete(objr.container.objects, objr.name)
	a.user.Objects--
	return nil
}

func (objr objectResource) post(a *action) interface{} {
	obj := objr.object
	obj.setMetadata(a, "object")
	return nil
}

func (objr objectResource) copy(a *action) interface{} {
	if objr.object == nil {
		fatalf(404, "NoSuchKey", "The specified key does not exist.")
	}

	obj := objr.object
	destination := a.req.Header.Get("Destination")
	if destination == "" {
		fatalf(400, "Bad Request", "You must provide a Destination header")
	}

	var (
		obj2  *object
		objr2 objectResource
	)

	destURL, _ := url.Parse("/auth/v1/AUTH_" + TEST_ACCOUNT + "/" + destination)
	r := a.srv.resourceForURL(destURL)
	switch t := r.(type) {
	case objectResource:
		objr2 = t
		if objr2.object == nil {
			obj2 = &object{
				name: objr2.name,
				metadata: metadata{
					meta: make(http.Header),
				},
			}
			a.user.Objects++
		} else {
			obj2 = objr2.object
			objr2.container.bytes -= len(obj2.data)
			a.user.BytesUsed -= int64(len(obj2.data))
		}
	default:
		fatalf(400, "Bad Request", "Destination must point to a valid object path")
	}

	obj2.content_type = obj.content_type
	obj2.data = obj.data
	obj2.checksum = obj.checksum
	obj2.mtime = time.Now()
	objr2.container.objects[objr2.name] = obj2
	objr2.container.bytes += len(obj.data)
	a.user.BytesUsed += int64(len(obj.data))

	for key, values := range obj.metadata.meta {
		obj2.metadata.meta[key] = values
	}
	obj2.setMetadata(a, "object")

	return nil
}
