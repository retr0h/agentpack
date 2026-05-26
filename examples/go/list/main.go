// Example: list installed packages programmatically.
package main

import (
	"fmt"
	"log"

	"github.com/retr0h/agentpack/pkg/list"
)

func main() {
	l := list.New()
	entries, err := l.Run()
	if err != nil {
		log.Fatal(err)
	}

	for _, e := range entries {
		fmt.Printf("%s  %s  %s  %s\n", e.Name, e.Version, e.SHA, e.Source)
	}
}
