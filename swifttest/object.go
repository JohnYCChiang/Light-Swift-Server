package swifttest

import (
	"fmt"
	"time"
)

type Object struct {
	Metadata
	Name         string
	Mtime        time.Time
	Checksum     []byte // also held as ETag in meta.
	Data         []byte
	Content_type string
}

func (obj *Object) Key() Key {
	return Key{
		Key:          obj.Name,
		LastModified: obj.Mtime.Format("2006-01-02T15:04:05"),
		Size:         int64(len(obj.Data)),
		ETag:         fmt.Sprintf("%x", obj.Checksum),
		ContentType:  obj.Content_type,
	}
}
