package swifttest

import (
	"sort"
	"strconv"
	"strings"
)

type rootResource struct{}

func (rootResource) put(a *action) interface{} { return notAllowed() }
func (rootResource) get(a *action) interface{} {
	marker := a.req.Form.Get("marker")
	prefix := a.req.Form.Get("prefix")
	format := a.req.URL.Query().Get("format")

	h := a.w.Header()

	h.Set("X-Account-Bytes-Used", strconv.Itoa(int(a.user.BytesUsed)))
	h.Set("X-Account-Container-Count", strconv.Itoa(int(a.user.Account.Containers)))
	h.Set("X-Account-Object-Count", strconv.Itoa(int(a.user.Objects)))

	// add metadata
	a.user.metadata.getMetadata(a)

	if a.req.Method == "HEAD" {
		return nil
	}

	var tmp orderedContainers
	// first get all matching objects and arrange them in alphabetical order.
	for _, container := range a.user.Containers {
		if strings.HasPrefix(container.name, prefix) {
			tmp = append(tmp, container)
		}
	}
	sort.Sort(tmp)

	resp := make([]Folder, 0)
	for _, container := range tmp {
		if container.name <= marker {
			continue
		}
		if format == "json" {
			resp = append(resp, Folder{
				Count: len(container.objects),
				Bytes: container.bytes,
				Name:  container.name,
			})
		} else {
			a.w.Write([]byte(container.name + "\n"))
		}
	}

	if format == "json" {
		return resp
	} else {
		return nil
	}
}

func (r rootResource) post(a *action) interface{} {
	a.user.metadata.setMetadata(a, "account")
	return nil
}

func (rootResource) delete(a *action) interface{} {
	if a.req.URL.Query().Get("bulk-delete") == "1" {
		fatalf(403, "Operation forbidden", "Bulk delete is not supported")
	}

	return notAllowed()
}

func (rootResource) copy(a *action) interface{} { return notAllowed() }
