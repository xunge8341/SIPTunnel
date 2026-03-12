package main

import (
	"fmt"
	"log"

	"siptunnel/internal/configdoc"
)

func main() {
	outputs, err := configdoc.GenerateAll(".")
	if err != nil {
		log.Fatalf("generate config docs failed: %v", err)
	}
	fmt.Printf("generated: %s\n", outputs.MarkdownPath)
	fmt.Printf("generated: %s\n", outputs.ExamplePath)
	fmt.Printf("generated: %s\n", outputs.DevPath)
	fmt.Printf("generated: %s\n", outputs.TestPath)
	fmt.Printf("generated: %s\n", outputs.ProdPath)
}
