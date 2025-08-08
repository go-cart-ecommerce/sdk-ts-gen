# GoCart TypeScript SDK Generator

This tool takes an OpenAPI specification as input and generates a TypeScript SDK with typed methods and parameters. It supports reading the OpenAPI document from a file or from standard input, and outputs the generated code to a specified directory.

## Prerequisites

- **Go 1.18+** (or a compatible version)
- **OpenAPI 3.x** specification file

## Installation

**Install directly from GitHub:**
```bash
go install github.com/go-cart-ecommerce/sdk-ts-gen@latest
```

This will install the `sdk-ts-gen` binary to your `$GOPATH/bin` directory (or `$GOBIN` if set).

## Usage

**Run the tool with a file:**
```bash
sdk-ts-gen -doc path/to/openapi.yaml -o ./gocart-sdk-ts
```
This reads the OpenAPI document from `path/to/openapi.yaml` and writes the generated files to `./gocart-sdk-ts/src`.

**Run the tool with standard input:**
```bash
cat openapi.yaml | sdk-ts-gen -doc - -o ./gocart-sdk-ts
```
This reads the OpenAPI document from `stdin` and writes the generated files to `./gocart-sdk-ts/src`.

**Check version:**
```bash
sdk-ts-gen -version
```

## Alternative: Build from Source

If you prefer to build from source:

```bash
go build -o openapi-to-sdk ./...
./openapi-to-sdk -doc path/to/openapi.yaml -o ./gocart-sdk-ts
```

## Flags

- `-doc`:  
  - Specify the OpenAPI document path.  
  - **Default:** Reads from `stdin` if not specified.

- `-o`:  
  - Specify the output directory for the generated files.
  - **Default:** `./src`

- `-version`:  
  - Show version information and exit.
