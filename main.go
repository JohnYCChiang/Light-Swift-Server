package main

import "fmt"
import "light-swift-server/swifttest"

func main() {
	_, err := swifttest.NewSwiftServer()
	if err != nil {
		return
	}
	fmt.Println("Server Start!")
	select {}
}
