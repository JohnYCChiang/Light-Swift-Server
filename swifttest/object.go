package swifttest

import (
	"fmt"
	"time"
)

type object struct {
	metadata
	name         string
	mtime        time.Time
	checksum     []byte // also held as ETag in meta.
	data         []byte
	content_type string
}

func (obj *object) Key() Key {
	return Key{
		Key:          obj.name,
		LastModified: obj.mtime.Format("2006-01-02T15:04:05"),
		Size:         int64(len(obj.data)),
		ETag:         fmt.Sprintf("%x", obj.checksum),
		ContentType:  obj.content_type,
	}
}
