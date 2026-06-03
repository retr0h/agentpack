// Example: install a package programmatically.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/retr0h/agentpack/pkg/install"
)

func main() {
	result, err := install.New().Run(context.Background(), install.Options{
		Source: "github.com/mukul975/Anthropic-Cybersecurity-Skills",
		Selectors: []install.ContentSelector{
			{Type: "skill", Name: "acquiring-disk-image-with-dd-and-dcfldd"},
		},
		OnStep: func(s install.Step) {
			fmt.Printf("  %s %s\n", s.Name, s.Detail)
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("installed %s (%s)\n", result.Name, result.SHA)
}
