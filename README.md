# GoCart TypeScript SDK Generator

This tool takes an OpenAPI specification as input and generates a TypeScript SDK with typed methods and parameters. It supports reading the OpenAPI document from a file or from standard input, and outputs the generated code to a specified directory.


## Prerequisites

- **Go 1.18+** (or a compatible version)
- **OpenAPI 3.x** specification file

## Usage

1. **Build the tool:**
   ```bash
   go build -o openapi-to-sdk ./...
   ```

2. **Run the tool with a file:**
   ```bash
   ./openapi-to-sdk -doc path/to/openapi.yaml -o ./gocart-sdk-ts
   ```
   This reads the OpenAPI document from `path/to/openapi.yaml` and writes the generated files to `./gocart-sdk-ts/src`.

3. **Run the tool with standard input:**
   ```bash
   cat openapi.yaml | ./openapi-to-sdk -doc - -o ./gocart-sdk-ts
   ```
   This reads the OpenAPI document from `stdin` and writes the generated files to `./gocart-sdk-ts/src`.

## Flags

- `-doc`:  
  - Specify the OpenAPI document path.  
  - **Default:** Reads from `stdin` if not specified.

- `-o`:  
  - Specify the output directory for the generated files.
  - **Default:** `./src`
