package main

import (
	"io/ioutil"
	"os"
	"strings"
)

func main() {
	if len(os.Args) != 4 {
		panic("usage: replace old new filename")
	}
	old := os.Args[1]
	new := os.Args[2]
	filename := os.Args[3]

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	newContent := strings.ReplaceAll(string(content), old, new)

	err = ioutil.WriteFile(filename, []byte(newContent), 0o644)
	if err != nil {
		panic(err)
	}
}
