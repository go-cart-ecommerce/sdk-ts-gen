package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

func generateTypes(doc *openapi3.T) []byte {
	typeBuf := bytes.Buffer{}
	typeBuf.WriteString("// Auto-generated TypeScript types\n\n")

	// Ensure consistent output order
	var schemaNames []string
	for name := range doc.Components.Schemas {
		schemaNames = append(schemaNames, name)
	}
	sort.Strings(schemaNames)

	for _, schemaName := range schemaNames {
		schemaRef := doc.Components.Schemas[schemaName]
		ts, _ := generateTypeScript(schemaName, schemaRef, doc)
		typeBuf.WriteString(ts + "\n\n")
	}

	return typeBuf.Bytes()
}

// generateTypeScript generates TypeScript interfaces/types from OpenAPI schemas
func generateTypeScript(name string, schemaRef *openapi3.SchemaRef, doc *openapi3.T) (string, error) {
	schema := schemaRef.Value
	if schema == nil {
		return "", fmt.Errorf("schema %s is nil", name)
	}

	// Handle enums
	if len(schema.Enum) > 0 {
		enumValues := make([]string, len(schema.Enum))
		for i, v := range schema.Enum {
			switch vv := v.(type) {
			case string:
				enumValues[i] = fmt.Sprintf(`"%s"`, vv)
			case float64:
				enumValues[i] = fmt.Sprintf(`%v`, vv)
			case bool:
				enumValues[i] = fmt.Sprintf(`%v`, vv)
			default:
				enumValues[i] = fmt.Sprintf(`%v`, vv)
			}
		}
		return fmt.Sprintf("export type %s = %s;", toPascalCase(name), strings.Join(enumValues, " | ")), nil
	}

	// Determine TypeScript type based on OpenAPI types
	tsType, _ := resolveType(schemaRef, doc)

	// If the resolved type is an object with properties, define an interface
	if isObject(schema) {
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("export interface %s {\n", toPascalCase(name)))

		// Collect property names to ensure consistent order
		var propNames []string
		for prop := range schema.Properties {
			propNames = append(propNames, prop)
		}
		sort.Strings(propNames)

		for _, propName := range propNames {
			// Skip the _embedded property; we'll handle it separately
			if propName == "_embedded" {
				continue
			}

			prop := schema.Properties[propName]
			optional := true
			if contains(schema.Required, propName) {
				optional = false
			}

			propType, _ := resolveType(prop, doc)

			camelPropName := toCamelCase(propName)

			if optional {
				buf.WriteString(fmt.Sprintf("  %s?: %s;\n", camelPropName, propType))
			} else {
				buf.WriteString(fmt.Sprintf("  %s: %s;\n", camelPropName, propType))
			}
		}

		// Handle _embedded properties by promoting them to top-level
		if embeddedSchemaRef, ok := schema.Properties["_embedded"]; ok && embeddedSchemaRef != nil {
			embeddedSchemaResolved, err := resolveSchemaRef(embeddedSchemaRef, doc)
			if err != nil {
				return "", fmt.Errorf("failed to resolve embedded $ref for %s: %v", name, err)
			}
			embeddedSchema := embeddedSchemaResolved.Value
			if embeddedSchema == nil {
				return "", fmt.Errorf("embedded schema %s is nil", embeddedSchemaRef.Ref)
			}

			for embeddedPropName, embeddedProp := range embeddedSchema.Properties {
				camelEmbeddedPropName := toCamelCase(embeddedPropName)
				embeddedPropType, _ := resolveType(embeddedProp, doc)
				optional := true
				if contains(embeddedSchema.Required, embeddedPropName) {
					optional = false
				}
				if optional {
					buf.WriteString(fmt.Sprintf("  %s?: %s;\n", camelEmbeddedPropName, embeddedPropType))
				} else {
					buf.WriteString(fmt.Sprintf("  %s: %s;\n", camelEmbeddedPropName, embeddedPropType))
				}
			}
		}

		buf.WriteString("}")
		return buf.String(), nil
	}

	// For other types (e.g., arrays, primitives), define a type alias
	return fmt.Sprintf("export type %s = %s;", toPascalCase(name), tsType), nil
}

// resolveSchemaRef resolves a $ref SchemaRef to the actual SchemaRef in the document
func resolveSchemaRef(schemaRef *openapi3.SchemaRef, doc *openapi3.T) (*openapi3.SchemaRef, error) {
	if schemaRef.Ref == "" {
		return schemaRef, nil
	}
	refName := getRefName(schemaRef.Ref)
	refSchema, ok := doc.Components.Schemas[refName]
	if !ok {
		return nil, fmt.Errorf("reference %s not found in components.schemas", refName)
	}
	return refSchema, nil
}

// isObject determines if the schema represents an object
func isObject(schema *openapi3.Schema) bool {
	if schema.Type != nil {
		for _, t := range *schema.Type {
			if strings.ToLower(t) == "object" {
				return true
			}
		}
	}
	if schema.Properties != nil && len(schema.Properties) > 0 {
		return true
	}
	return false
}

// isArray determines if the schema represents an array
func isArray(schema *openapi3.Schema) bool {
	if schema.Type != nil {
		for _, t := range *schema.Type {
			if strings.ToLower(t) == "array" {
				return true
			}
		}
	}
	return false
}

// contains checks if a slice contains a string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// removeDuplicates removes duplicate strings from a slice
func removeDuplicates(elements []string) []string {
	encountered := map[string]bool{}
	result := []string{}
	for _, v := range elements {
		if !encountered[v] {
			encountered[v] = true
			result = append(result, v)
		}
	}
	return result
}
