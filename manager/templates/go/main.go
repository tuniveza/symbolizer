package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello from __NAME__", os.Args[1:])
}
