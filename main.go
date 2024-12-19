package main

import (
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3"
)

var (
	docPath   string
	outputDir string
)

func init() {
	flag.StringVar(&docPath, "doc", "-", "Path to the OpenAPI document file. Use '-' to read from stdin.")
	flag.StringVar(&outputDir, "o", "./src", "Output directory where the generated files will be placed.")
}

func main() {
	flag.Parse()

	var docData []byte
	var err error

	if docPath == "-" {
		// Read from stdin
		docData, err = io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("Failed to read OpenAPI spec from stdin: %v", err)
		}
	} else {
		// Read from file
		docData, err = os.ReadFile(docPath)
		if err != nil {
			log.Fatalf("Failed to read OpenAPI spec from file %s: %v", docPath, err)
		}
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	doc, err := loader.LoadFromData(docData)
	if err != nil {
		log.Fatalf("Failed to load OpenAPI document: %v", err)
	}

	// Generate code
	sdkBytes := generateSDK(doc)
	typesBytes := generateTypes(doc)
	paramBytes := generateParams(doc)

	// Ensure output directory structure
	srcDir := filepath.Join(outputDir)
	err = os.MkdirAll(srcDir, 0755)
	if err != nil {
		log.Fatalf("Failed to create output directory %s: %v", srcDir, err)
	}

	// Write files
	writeFile(filepath.Join(srcDir, "sdk.ts"), sdkBytes)
	writeFile(filepath.Join(srcDir, "types.ts"), typesBytes)
	writeFile(filepath.Join(srcDir, "params.ts"), paramBytes)

	log.Printf("Hey! Generated TypeScript SDK in %s\n", outputDir)
}

func writeFile(path string, data []byte) {
	err := os.WriteFile(path, data, 0644)
	if err != nil {
		log.Fatalf("Failed to write %s: %v", path, err)
	}
}
