package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

func generateParams(doc *openapi3.T) []byte {
	// Prepare to collect all TypeScript types
	var tsBuffer bytes.Buffer
	tsBuffer.WriteString("// Auto-generated TypeScript types\n\n")

	var allAdditionalTypes []string

	// Iterate over all paths in matching order
	for _, path := range doc.Paths.InMatchingOrder() {
		pathItem := doc.Paths.Find(path)

		// Iterate over all operations in the path
		operations := map[string]*openapi3.Operation{
			"GET": pathItem.Get,
		}

		for method, operation := range operations {
			if operation == nil {
				continue
			}

			// Determine a unique interface name
			interfaceName := generateInterfaceName(method, path, operation.OperationID)

			// Extract query parameters
			queryParams := extractQueryParameters(operation)

			// Process parameters into TypeScript interface
			tsInterface, additionalTypes := generateTypeScriptInterface(interfaceName, queryParams, operation, doc)

			// Collect all additional TypeScript types (e.g., enums)
			allAdditionalTypes = append(allAdditionalTypes, additionalTypes...)

			// Append the generated interface to the buffer
			tsBuffer.WriteString(tsInterface)
			tsBuffer.WriteString("\n")
		}
	}

	// Sort additional types to ensure consistent output
	sort.Strings(allAdditionalTypes)
	for _, tsType := range allAdditionalTypes {
		tsBuffer.WriteString(tsType + "\n\n")
	}

	// Output the TypeScript code
	return tsBuffer.Bytes()
}

// generateInterfaceName creates a unique and descriptive TypeScript interface name
func generateInterfaceName(method, path, operationID string) string {
	if operationID != "" {
		return toPascalCase(operationID) + "Params"
	}

	// Replace non-alphanumeric characters with underscores
	cleanPath := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, path)

	return toPascalCase(method) + toPascalCase(cleanPath) + "Params"
}

// extractQueryParameters extracts query parameters from an operation
func extractQueryParameters(operation *openapi3.Operation) []Parameter {
	var params []Parameter
	for _, paramRef := range operation.Parameters {
		param := paramRef.Value
		if param.In == "query" {
			params = append(params, Parameter{
				Name:        param.Name,
				In:          param.In,
				Description: param.Description,
				Schema:      param.Schema,
				Required:    param.Required,
			})
		}
	}
	return params
}

// generateTypeScriptInterface generates the TypeScript interface for parameters
func generateTypeScriptInterface(interfaceName string, params []Parameter, operation *openapi3.Operation, doc *openapi3.T) (string, []string) {
	var buf bytes.Buffer
	var additionalTypes []string

	// Start interface
	buf.WriteString(fmt.Sprintf("export interface %s {\n", interfaceName))

	// Group parameters by their base (e.g., filter, page)
	groupedParams := groupParameters(params)

	// Sort the group names for consistent output
	groupNames := make([]string, 0, len(groupedParams))
	for groupName := range groupedParams {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)

	// Iterate over grouped parameters
	for _, groupName := range groupNames {
		group := groupedParams[groupName]

		// Generate comments
		groupDesc := fmt.Sprintf("%s for the API.", strings.Title(groupName))
		buf.WriteString(fmt.Sprintf("  /**\n   * %s\n   */\n", groupDesc))

		// Generate nested interface or type
		var nestedType string
		var nestedAdditionalTypes []string

		if groupName == "include" || groupName == "sort" {
			// Handle enumerated types with prefix based on interface name
			enumTypeName := interfaceName + toPascalCase(groupName) + "Option"

			enumValues := extractEnumValues(groupName, group)
			if len(enumValues) > 0 {
				// Create TypeScript type for enum
				tsType := fmt.Sprintf("type %s = %s;", enumTypeName, strings.Join(enumValues, " | "))
				additionalTypes = append(additionalTypes, tsType)
				// Define the property as an array of the enum type
				nestedType = fmt.Sprintf("%s[]", enumTypeName)
			} else {
				// Fallback to string array if no enum values are found
				nestedType = "string[]"
			}
		} else {
			// Handle nested objects like filter and page
			nestedType, nestedAdditionalTypes = generateNestedInterface(strings.Title(groupName), group, doc)
			additionalTypes = append(additionalTypes, nestedAdditionalTypes...)
		}

		// Add to main interface
		buf.WriteString(fmt.Sprintf("  %s?: %s;\n\n", toCamelCase(groupName), nestedType))
	}

	if strings.HasPrefix(operation.OperationID, "list") {
		// Add to main interface
		buf.WriteString("  /**\n")
		buf.WriteString("   * Include the count of total items in the collection.\n")
		buf.WriteString("   */\n")
		buf.WriteString("  totalCount?: boolean;\n")
	}

	// Close interface
	buf.WriteString("}\n")

	return buf.String(), additionalTypes
}

// groupParameters groups parameters by their base (e.g., filter, page)
func groupParameters(params []Parameter) map[string][]Parameter {
	grouped := make(map[string][]Parameter)
	for _, param := range params {
		// Check if parameter name has a nested structure like filter[id]
		if strings.Contains(param.Name, "[") && strings.HasSuffix(param.Name, "]") {
			base := param.Name[:strings.Index(param.Name, "[")]
			nested := param.Name[strings.Index(param.Name, "[")+1 : len(param.Name)-1]
			// Create a new parameter with base as group and nested name
			grouped[base] = append(grouped[base], Parameter{
				Name:        nested,
				In:          param.In,
				Description: param.Description,
				Schema:      param.Schema,
				Required:    param.Required,
			})
		} else {
			// Treat as top-level parameter
			grouped[param.Name] = append(grouped[param.Name], param)
		}
	}
	return grouped
}

// extractEnumValues extracts enum values for specific groups
func extractEnumValues(groupName string, params []Parameter) []string {
	var enumValues []string
	for _, param := range params {
		if param.Schema != nil && len(param.Schema.Value.Enum) > 0 {
			for _, enumVal := range param.Schema.Value.Enum {
				if strVal, ok := enumVal.(string); ok {
					enumValues = append(enumValues, fmt.Sprintf(`'%s'`, toCamelCase(strVal)))
				}
			}
		} else {
			// TODO: fix this
			// Handle 'include' parameter without explicit enum by inferring from predefined values
			if groupName == "include" {
				enumValues = []string{"'parent'", "'parents'", "'children'", "'attributes'"}
			}
		}
	}
	// Remove duplicates
	enumValues = removeDuplicates(enumValues)
	return enumValues
}

// generateNestedInterface generates a TypeScript nested interface or type
func generateNestedInterface(name string, params []Parameter, doc *openapi3.T) (string, []string) {
	var buf bytes.Buffer
	var additionalTypes []string

	buf.WriteString("{\n")
	for _, param := range params {
		// Add comments
		buf.WriteString(fmt.Sprintf("    /**\n     * %s\n     */\n", param.Description))

		// Determine TypeScript type
		tsType, additional := resolveType(param.Schema, doc)
		additionalTypes = append(additionalTypes, additional...)

		// Handle nullable fields
		if param.Schema != nil && param.Schema.Value.Nullable {
			tsType = fmt.Sprintf("%s | null", tsType)
		}

		// Mark as optional if not required
		optional := "?:"
		if param.Required {
			optional = ":"
		}

		// Add to buffer with correct formatting
		buf.WriteString(fmt.Sprintf("    %s%s %s;\n\n", toCamelCase(param.Name), optional, tsType))
	}
	buf.WriteString("  }")

	return buf.String(), additionalTypes
}

// resolveType resolves the TypeScript type from an OpenAPI SchemaRef
func resolveType(schemaRef *openapi3.SchemaRef, doc *openapi3.T) (string, []string) {
	var additionalTypes []string
	if schemaRef == nil || schemaRef.Value == nil {
		return "any", additionalTypes
	}

	// Handle $ref
	if schemaRef.Ref != "" {
		refName := getRefName(schemaRef.Ref)
		tsType := toPascalCase(refName)
		return tsType, additionalTypes
	}

	schema := schemaRef.Value

	// Handle enums
	if len(schema.Enum) > 0 {
		enumValues := []string{}
		for _, enumVal := range schema.Enum {
			if strVal, ok := enumVal.(string); ok {
				enumValues = append(enumValues, fmt.Sprintf(`'%s'`, toCamelCase(strVal)))
			} else {
				enumValues = append(enumValues, fmt.Sprintf(`%v`, enumVal))
			}
		}
		tsType := strings.Join(enumValues, " | ")
		return tsType, additionalTypes
	}

	// Handle multiple types (union types)
	if schema.Type != nil && len(*schema.Type) > 0 {
		tsTypes := []string{}
		for _, t := range *schema.Type {
			switch strings.ToLower(t) {
			case "integer", "number", "string", "boolean", "object", "array", "null":
				if mappedType, ok := typeMapping[strings.ToLower(t)]; ok {
					if strings.ToLower(t) == "array" && schema.Items != nil {
						// Recursively resolve the type of array items
						itemType, additional := resolveType(schema.Items, doc)
						additionalTypes = append(additionalTypes, additional...)
						tsTypes = append(tsTypes, fmt.Sprintf("%s[]", itemType))
					} else {
						tsTypes = append(tsTypes, mappedType)
					}
				} else {
					tsTypes = append(tsTypes, "any")
				}
			default:
				tsTypes = append(tsTypes, "any")
			}
		}
		// Remove duplicate types
		tsTypes = removeDuplicates(tsTypes)
		// Join with | for union types
		return strings.Join(tsTypes, " | "), additionalTypes
	}

	// Handle single type
	if schema.Type != nil && len(*schema.Type) == 1 {
		t := (*schema.Type)[0]
		switch strings.ToLower(t) {
		case "integer", "number", "string", "boolean", "object", "array", "null":
			if mappedType, ok := typeMapping[strings.ToLower(t)]; ok {
				if strings.ToLower(t) == "array" && schema.Items != nil {
					// Recursively resolve the type of array items
					itemType, additional := resolveType(schema.Items, doc)
					additionalTypes = append(additionalTypes, additional...)
					return fmt.Sprintf("%s[]", itemType), additionalTypes
				}

				// Handle special formats
				if strings.ToLower(t) == "string" {
					// Handle special formats
					if t == "string" {
						// Map specific string formats to string or more specific types if desired
						if schema.Format == "date-time" || schema.Format == "uuid" {
							return "string", additionalTypes
						}
					}
				}

				return mappedType, additionalTypes
			}
			return "any", additionalTypes
		default:
			return "any", additionalTypes
		}
	}

	// Default fallback
	return "any", additionalTypes
}
