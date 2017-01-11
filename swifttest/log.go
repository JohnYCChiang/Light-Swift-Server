package swifttest

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
)

func fatalf(code int, codeStr string, errf string, a ...interface{}) {
	if DEBUG {
		log.Printf("statusCode %q Code %s Message %s ", code, codeStr, fmt.Sprintf(errf, a...))
	}
	panic(&swiftError{
		statusCode: code,
		Code:       codeStr,
		Message:    fmt.Sprintf(errf, a...),
	})
}

func jsonMarshal(w io.Writer, x interface{}) {
	if err := json.NewEncoder(w).Encode(x); err != nil {
		panic(fmt.Errorf("error marshalling %#v: %v", x, err))
	}
}
