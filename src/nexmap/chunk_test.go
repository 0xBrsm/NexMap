//go:build ignore

package main

import "fmt"

func main() {
	lib, err := LoadChunkLibrary()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	lib.PrintLibrarySummary()
}
