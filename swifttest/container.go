package swifttest

import (
	"path"
	"sort"
	"strings"
	"time"
)

// The Key type represents an item stored in an container.
type Key struct {
	Key          string `json:"name"`
	LastModified string `json:"last_modified"`
	Size         int64  `json:"bytes"`
	// ETag gives the hex-encoded MD5 sum of the contents,
	// surrounded with double-quotes.
	ETag        string `json:"hash"`
	ContentType string `json:"content_type"`
	// Owner        Owner
}

type container struct {
	metadata
	name    string
	ctime   time.Time
	objects map[string]*object
	bytes   int
}

func (c container) list(delimiter string, marker string, prefix string, parent string) (resp []interface{}) {
	var tmp orderedObjects

	// first get all matching objects and arrange them in alphabetical order.
	for _, obj := range c.objects {
		if strings.HasPrefix(obj.name, prefix) {
			tmp = append(tmp, obj)
		}
	}
	sort.Sort(tmp)

	var prefixes []string
	for _, obj := range tmp {
		if !strings.HasPrefix(obj.name, prefix) {
			continue
		}

		isPrefix := false
		name := obj.name
		if parent != "" {
			if path.Dir(obj.name) != path.Clean(parent) {
				continue
			}
		} else if delimiter != "" {
			if i := strings.Index(obj.name[len(prefix):], delimiter); i >= 0 {
				name = obj.name[:len(prefix)+i+len(delimiter)]
				if prefixes != nil && prefixes[len(prefixes)-1] == name {
					continue
				}
				isPrefix = true
			}
		}

		if name <= marker {
			continue
		}

		if isPrefix {
			prefixes = append(prefixes, name)

			resp = append(resp, Subdir{
				Subdir: name,
			})
		} else {
			resp = append(resp, obj)
		}
	}

	return
}

// validContainerName returns whether name is a valid bucket name.
// Here are the rules, from:
// http://docs.openstack.org/api/openstack-object-storage/1.0/content/ch_object-storage-dev-api-storage.html
//
// Container names cannot exceed 256 bytes and cannot contain the / character.
//
func validContainerName(name string) bool {
	if len(name) == 0 || len(name) > 256 {
		return false
	}
	for _, r := range name {
		switch {
		case r == '/':
			return false
		default:
		}
	}
	return true
}
