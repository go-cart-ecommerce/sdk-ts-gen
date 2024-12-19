package main

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

// TypeMapping maps OpenAPI types to TypeScript types
var typeMapping = map[string]string{
	"integer": "number",
	"number":  "number",
	"string":  "string",
	"boolean": "boolean",
	"object":  "any",
	"array":   "", // Special handling for arrays
	"null":    "null",
}

type methodParam struct {
	Name string
	Type string
}

type methodParams []methodParam

func (m methodParams) HasParam(name string) bool {
	for _, p := range m {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (m methodParams) GetPayloadParam() (methodParam, bool) {
	for _, p := range m {
		if p.Name == "req" {
			return p, true
		}
	}
	return methodParam{}, false
}

func generateSDK(doc *openapi3.T) []byte {
	// Prepare to collect all TypeScript methods and types
	var tsBuffer bytes.Buffer

	// We'll store the methods in a slice for sorting by method name
	type generatedMethod struct {
		Name string
		Code string
	}
	var allMethods []generatedMethod

	// Sets to store unique TypeScript types to import
	typeSet := make(map[string]struct{})
	paramsSet := make(map[string]struct{})

	// Iterate over all paths
	for _, path := range doc.Paths.InMatchingOrder() {
		pathItem := doc.Paths.Find(path)

		// Iterate over all operations in the path
		operations := map[string]*openapi3.Operation{
			"GET":    pathItem.Get,
			"POST":   pathItem.Post,
			"PATCH":  pathItem.Patch,
			"PUT":    pathItem.Put,
			"DELETE": pathItem.Delete,
			// Add other HTTP methods if necessary
		}

		for method, operation := range operations {
			if operation == nil {
				continue
			}

			// Determine if the method has a request body
			hasRequestBody := false
			var requestType string
			if operation.RequestBody != nil && operation.RequestBody.Value != nil {
				content := operation.RequestBody.Value.Content
				if appJSON, ok := content["application/json"]; ok && appJSON.Schema != nil {
					if appJSON.Schema.Ref != "" {
						requestType = toPascalCase(getRefName(appJSON.Schema.Ref))
					} else {
						// Handle inline schemas or other types
						requestType = resolveInlineType(appJSON.Schema, typeSet)
					}
					if !isPrimitiveType(requestType) && requestType != "any" {
						typeSet[requestType] = struct{}{}
					}
					hasRequestBody = true
				}
			}

			// Determine method name
			methodName := generateMethodName(operation, method, path)

			// Extract parameters
			queryParams := extractParameters(operation)

			var methodParamsList methodParams

			// Determine parameter type and name based on HTTP method
			switch strings.ToUpper(method) {
			case "POST", "PUT":
				if hasRequestBody {
					paramTypeName := requestType
					methodParamsList = append(methodParamsList, methodParam{Name: "req", Type: paramTypeName})
					typeSet[paramTypeName] = struct{}{}
				} else {
					// Fallback if no request body is present
					paramTypeName := toPascalCase(methodName) + "Params"
					methodParamsList = append(methodParamsList, methodParam{Name: "params", Type: paramTypeName})
					paramsSet[paramTypeName] = struct{}{}
				}
			case "PATCH":
				// Assume PATCH methods have an 'id' parameter and a request body
				methodParamsList = append(methodParamsList, methodParam{Name: "id", Type: "string"})
				paramTypeName := requestType
				methodParamsList = append(methodParamsList, methodParam{Name: "req", Type: paramTypeName})
				typeSet[paramTypeName] = struct{}{}
			case "DELETE":
				// Assume DELETE methods only have a single string parameter
				methodParamsList = append(methodParamsList, methodParam{Name: "id", Type: "string"})
			case "GET":
				// check if it is getOne or getAll
				if !strings.Contains(methodName, "list") {
					methodParamsList = append(methodParamsList, methodParam{Name: "id", Type: "string"})
				}
				paramTypeName := toPascalCase(methodName) + "Params"
				methodParamsList = append(methodParamsList, methodParam{Name: "params", Type: paramTypeName})
				paramsSet[paramTypeName] = struct{}{}
			default:
				fmt.Println("unsupported method:", method)
			}

			// Collect parameter types (for GET, DELETE, etc.)
			if methodParamsList.HasParam("params") && queryParams["query"] != nil {
				for _, p := range queryParams["query"] {
					// If the query parameter references a schema, add it to paramsSet
					if p.Schema != nil && p.Schema.Ref != "" {
						tsType := toPascalCase(getRefName(p.Schema.Ref))
						paramsSet[tsType] = struct{}{}
					}
				}
			}

			// Determine response type
			responseType := determineResponseType(operation, typeSet)
			if !isPrimitiveType(responseType) && !isArrayType(responseType) && responseType != "any" {
				typeSet[responseType] = struct{}{}
			}

			// Generate method
			methodCode := generateMethod(doc, methodName, method, path, methodParamsList, responseType, queryParams)

			allMethods = append(allMethods, generatedMethod{
				Name: methodName,
				Code: methodCode,
			})
		}
	}

	// Sort methods alphabetically by method name
	sort.Slice(allMethods, func(i, j int) bool {
		return allMethods[i].Name < allMethods[j].Name
	})

	// Generate import statements with collected types
	importTypes := []string{}
	for tsType := range typeSet {
		if !isPrimitiveType(tsType) && !isArrayType(tsType) && tsType != "any" {
			importTypes = append(importTypes, tsType)
		}
	}
	sort.Strings(importTypes)

	importParams := []string{}
	for tsType := range paramsSet {
		if !isPrimitiveType(tsType) && !isArrayType(tsType) && tsType != "any" {
			importParams = append(importParams, tsType)
		}
	}
	sort.Strings(importParams)

	tsBuffer.WriteString("// Auto-generated TypeScript SDK\n")
	tsBuffer.WriteString("// Do not modify manually.\n\n")

	// Generate import statement for types.ts
	if len(importTypes) > 0 {
		tsBuffer.WriteString("import {\n")
		for _, tsType := range importTypes {
			tsBuffer.WriteString(fmt.Sprintf("  %s,\n", tsType))
		}
		tsBuffer.WriteString("} from './types';\n")
	}

	// Generate import statement for params.ts
	if len(importParams) > 0 {
		if len(importTypes) > 0 {
			tsBuffer.WriteString("\n")
		}
		tsBuffer.WriteString("import {\n")
		for _, tsType := range importParams {
			tsBuffer.WriteString(fmt.Sprintf("  %s,\n", tsType))
		}
		tsBuffer.WriteString("} from './params';\n")
	}

	tsBuffer.WriteString("import { toApiType, toClientType } from './utils';\n\n")

	// Start GoCartSDK class
	tsBuffer.WriteString("export class GoCartSDK {\n")
	tsBuffer.WriteString("  private baseUrl: string;\n\n")
	tsBuffer.WriteString("  constructor(baseUrl: string = 'https://api.orbita.al') {\n")
	tsBuffer.WriteString("    this.baseUrl = baseUrl;\n")
	tsBuffer.WriteString("  }\n\n")

	// Append all sorted methods
	for _, m := range allMethods {
		tsBuffer.WriteString(m.Code)
		tsBuffer.WriteString("\n")
	}

	// Close GoCartSDK class
	tsBuffer.WriteString("}\n")

	return tsBuffer.Bytes()
}

func isArrayType(tsType string) bool {
	return strings.HasSuffix(tsType, "[]")
}

// generateMethodName creates a TypeScript method name based on operationId or HTTP method and path
func generateMethodName(operation *openapi3.Operation, method, path string) string {
	return operation.OperationID
}

// extractParameters separates path, query, and body parameters
func extractParameters(operation *openapi3.Operation) map[string][]Parameter {
	groupedParams := make(map[string][]Parameter)

	for _, paramRef := range operation.Parameters {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		param := paramRef.Value
		p := Parameter{
			Name:        param.Name,
			In:          param.In,
			Description: param.Description,
			Schema:      param.Schema,
			Required:    param.Required,
		}
		groupedParams[param.In] = append(groupedParams[param.In], p)
	}

	// Handle requestBody if exists
	if operation.RequestBody != nil && operation.RequestBody.Value != nil {
		content := operation.RequestBody.Value.Content
		if appJSON, ok := content["application/json"]; ok && appJSON.Schema != nil {
			p := Parameter{
				Name:        "body",
				In:          "body",
				Description: "Request body",
				Schema:      appJSON.Schema,
				Required:    operation.RequestBody.Value.Required,
			}
			groupedParams["body"] = append(groupedParams["body"], p)
		}
	}

	return groupedParams
}

// determineResponseType selects the appropriate response type from the operation's responses
func determineResponseType(operation *openapi3.Operation, typeSet map[string]struct{}) string {
	// Prioritize 200, then 201, etc.
	priority := []int{200, 201, 202, 204}

	for _, code := range priority {
		respRef := operation.Responses.Status(code)
		if respRef != nil && respRef.Value != nil {
			if respRef.Value.Content != nil {
				if appJSON, ok := respRef.Value.Content["application/json"]; ok && appJSON.Schema != nil {
					// Resolve schema reference if exists
					schema := appJSON.Schema
					if schema.Ref != "" {
						return toPascalCase(getRefName(schema.Ref))
					} else {
						// Handle inline schemas or other types
						return resolveInlineType(schema, typeSet)
					}
				}
			}
		}
	}

	// Fallback to 'any' if no suitable response found
	return "any"
}

// resolveInlineType resolves TypeScript types from inline OpenAPI schemas
func resolveInlineType(schema *openapi3.SchemaRef, typeSet map[string]struct{}) string {
	if schema.Value.Type != nil {
		if len(*schema.Value.Type) == 1 {
			openType := strings.ToLower((*schema.Value.Type)[0])
			if openType == "array" {
				// Handle array types
				if schema.Value.Items != nil {
					itemType := ""
					if schema.Value.Items.Ref != "" {
						itemType = toPascalCase(getRefName(schema.Value.Items.Ref))
						if !isPrimitiveType(itemType) && itemType != "any" {
							typeSet[itemType] = struct{}{}
						}
					} else {
						itemType = resolveInlineType(schema.Value.Items, typeSet)
					}
					return fmt.Sprintf("%s[]", itemType)
				}
				// Default to any[] if items are undefined
				return "any[]"
			}

			if tsType, ok := typeMapping[openType]; ok {
				// Handle special cases like enums
				if openType == "enum" && len(schema.Value.Enum) > 0 {
					enumValues := []string{}
					for _, enumVal := range schema.Value.Enum {
						if strVal, ok := enumVal.(string); ok {
							enumValues = append(enumValues, fmt.Sprintf(`'%s'`, strVal))
						} else {
							enumValues = append(enumValues, fmt.Sprintf(`%v`, enumVal))
						}
					}
					return strings.Join(enumValues, " | ")
				}
				return tsType
			}

			// handle date-time
			if openType == "string" && schema.Value.Format == "date-time" {
				return "Date"
			}
		} else {
			// Handle union types
			types := []string{}
			for _, t := range *schema.Value.Type {
				openType := strings.ToLower(t)
				if openType == "array" && schema.Value.Items != nil {
					itemType := ""
					if schema.Value.Items.Ref != "" {
						itemType = toPascalCase(getRefName(schema.Value.Items.Ref))
						if !isPrimitiveType(itemType) && itemType != "any" {
							typeSet[itemType] = struct{}{}
						}
					} else {
						itemType = resolveInlineType(schema.Value.Items, typeSet)
					}
					types = append(types, fmt.Sprintf("%s[]", itemType))
				} else if tsType, ok := typeMapping[openType]; ok {
					// Handle enums and other types
					if openType == "enum" && len(schema.Value.Enum) > 0 {
						enumValues := []string{}
						for _, enumVal := range schema.Value.Enum {
							if strVal, ok := enumVal.(string); ok {
								enumValues = append(enumValues, fmt.Sprintf(`'%s'`, strVal))
							} else {
								enumValues = append(enumValues, fmt.Sprintf(`%v`, enumVal))
							}
						}
						types = append(types, strings.Join(enumValues, " | "))
					} else {
						types = append(types, tsType)
					}
				}
			}
			return strings.Join(types, " | ")
		}
	}

	// Handle enums without 'enum' type
	if len(schema.Value.Enum) > 0 {
		enumValues := []string{}
		for _, enumVal := range schema.Value.Enum {
			if strVal, ok := enumVal.(string); ok {
				enumValues = append(enumValues, fmt.Sprintf(`'%s'`, strVal))
			} else {
				enumValues = append(enumValues, fmt.Sprintf(`%v`, enumVal))
			}
		}
		return strings.Join(enumValues, " | ")
	}

	// Fallback to 'any'
	return "any"
}

// generateMethod creates a TypeScript method within the GoCartSDK class
func generateMethod(doc *openapi3.T, methodName, httpMethod, path string, methodParamsList methodParams, responseType string, params map[string][]Parameter) string {
	var buf bytes.Buffer

	// Generate JSDoc comments
	buf.WriteString("  /**\n")
	buf.WriteString(fmt.Sprintf("   * %s\n", methodName))
	for _, p := range methodParamsList {
		buf.WriteString(fmt.Sprintf("   * @param %s %s\n", p.Name, p.Type))
	}
	buf.WriteString(fmt.Sprintf("   * @returns Promise<%s>\n", responseType))
	buf.WriteString("   */\n")

	// Generate method signature
	paramsSignature := []string{}
	for _, p := range methodParamsList {
		paramsSignature = append(paramsSignature, fmt.Sprintf("%s: %s", p.Name, p.Type))
	}
	buf.WriteString(fmt.Sprintf("  public async %s(%s): Promise<%s> {\n", methodName, strings.Join(paramsSignature, ", "), responseType))

	// Construct URL with path parameters
	url := path
	pathParams := extractPathParams(path)
	if len(pathParams) > 0 {
		for _, p := range pathParams {
			camelParam := toCamelCase(p)
			url = strings.ReplaceAll(url, "{"+p+"}", fmt.Sprintf("${%s}", camelParam))
		}
		buf.WriteString(fmt.Sprintf("    const url = `${this.baseUrl}%s`;\n", url))
	} else {
		buf.WriteString(fmt.Sprintf("    const url = `${this.baseUrl}%s`;\n", url))
	}

	if httpMethod == "POST" || httpMethod == "PUT" || httpMethod == "PATCH" {
		// cerate an array with keys of embedded objects in request body type
		var embeddedObjects []string
		if param, ok := methodParamsList.GetPayloadParam(); ok {
			if schemaRef, exists := doc.Components.Schemas[param.Type]; exists && schemaRef.Value != nil {
				// Look for the `_embedded` property
				if embeddedSchema, exists := schemaRef.Value.Properties["_embedded"]; exists && embeddedSchema.Value != nil {
					// Iterate over properties within `_embedded`
					for propName, propSchema := range embeddedSchema.Value.Properties {
						// Check if the property is an object (excluding arrays and primitives)
						if propSchema.Value != nil && propSchema.Value.Type.Is("object") && propSchema.Value.Items == nil {
							embeddedObjects = append(embeddedObjects, toCamelCase(propName))
						}
					}
				}
			}
		}

		// write it
		var bufEmbedded bytes.Buffer
		bufEmbedded.WriteString("[")
		for i, eo := range embeddedObjects {
			bufEmbedded.WriteString(fmt.Sprintf("'%s'", eo))
			if i < len(embeddedObjects)-1 {
				bufEmbedded.WriteString(", ")
			}
		}
		bufEmbedded.WriteString("]")
		buf.WriteString(fmt.Sprintf("		const embeddedObjects: string[] = %v;\n", bufEmbedded.String()))
		if param, ok := methodParamsList.GetPayloadParam(); ok {
			buf.WriteString(fmt.Sprintf("		const body = toApiType(%s, embeddedObjects);\n", param.Name))
		}

		// Initialize options for fetch
		buf.WriteString("    let options: RequestInit = {\n")
		buf.WriteString(fmt.Sprintf("      method: '%s',\n", strings.ToUpper(httpMethod)))
		buf.WriteString("      headers: {\n")
		buf.WriteString("        'Content-Type': 'application/json',\n")
		buf.WriteString("        // Add other headers like authentication here\n")
		buf.WriteString("      },\n")

		if _, ok := methodParamsList.GetPayloadParam(); ok {
			buf.WriteString("      body: JSON.stringify(body),\n")
		}
	} else {
		// Initialize options for fetch
		buf.WriteString("    let options: RequestInit = {\n")
		buf.WriteString(fmt.Sprintf("      method: '%s',\n", strings.ToUpper(httpMethod)))
		buf.WriteString("      headers: {\n")
		buf.WriteString("        'Content-Type': 'application/json',\n")
		buf.WriteString("        // Add other headers like authentication here\n")
		buf.WriteString("      },\n")
	}

	buf.WriteString("    };\n")

	if strings.HasPrefix(methodName, "list") {
		buf.WriteString("    if (params.totalCount) {\n")
		buf.WriteString("      options.headers = {\n")
		buf.WriteString("        ...options.headers,\n")
		buf.WriteString("        'Collection-Total': 'include'\n")
		buf.WriteString("      }\n")
		buf.WriteString("    }\n")
	}

	// Handle query parameters (only for methods that can have query params, typically GET, DELETE)
	// Assuming that methods with 'params' can have query parameters
	if methodParamsList.HasParam("params") && params["query"] != nil {
		paramName := "params"

		buf.WriteString("    const queryString = new URLSearchParams();\n")

		// Separate filter, sort, and page parameters
		var filterParams, sortParams, pageParams, includeParams []Parameter
		for _, qp := range params["query"] {
			switch {
			case strings.HasPrefix(qp.Name, "filter["):
				filterParams = append(filterParams, Parameter{
					Name:        toCamelCase(qp.Name),
					In:          "query",
					Description: qp.Description,
					Schema:      qp.Schema,
					Required:    qp.Required,
				})
			case qp.Name == "sort":
				sortParams = append(sortParams, qp)
			case strings.HasPrefix(qp.Name, "page["):
				pageParams = append(pageParams, Parameter{
					Name:        stripPageParams(qp.Name),
					In:          "query",
					Description: qp.Description,
					Schema:      qp.Schema,
					Required:    qp.Required,
				})
			case qp.Name == "include":
				includeParams = append(includeParams, qp)
			default:
				// Handle other query parameters if any
			}
		}

		// Handle filter parameters
		if len(filterParams) > 0 {
			buf.WriteString(fmt.Sprintf("    if (%s.filter) {\n", paramName))
			for _, fp := range filterParams {
				// Extract key from 'filter[key]'
				parent, key := parseBracketParam(fp.Name)
				if parent == "" || key == "" {
					// Fallback to regular access
					camelQP := toCamelCase(fp.Name)
					buf.WriteString(fmt.Sprintf("      if (%s.%s !== undefined && %s.%s !== null) {\n", paramName, camelQP, paramName, camelQP))
					buf.WriteString(fmt.Sprintf("        queryString.append('%s', String(%s.%s));\n", fp.Name, paramName, camelQP))
					buf.WriteString("      }\n")
					continue
				}

				camelParent := toCamelCase(parent)
				// camelKey := toCamelCase(key) // Not used in this context
				buf.WriteString(fmt.Sprintf("      if (%s.%s[\"%s\"] !== undefined && %s.%s[\"%s\"] !== null) {\n", paramName, camelParent, key, paramName, camelParent, key))
				buf.WriteString(fmt.Sprintf("        queryString.append('%s', String(%s.%s[\"%s\"]));\n", fp.Name, paramName, camelParent, key))
				buf.WriteString("      }\n")
			}
			buf.WriteString("    }\n")
		}

		// Handle sort parameters
		if len(sortParams) > 0 {
			for _, sp := range sortParams {
				camelSP := toCamelCase(sp.Name)
				buf.WriteString(fmt.Sprintf("    if (%s.%s !== undefined && %s.%s !== null) {\n", paramName, camelSP, paramName, camelSP))
				buf.WriteString(fmt.Sprintf("      queryString.append('%s', String(%s.%s));\n", sp.Name, paramName, camelSP))
				buf.WriteString("    }\n")
			}
		}

		// Handle pagination parameters
		if len(pageParams) > 0 {
			buf.WriteString(fmt.Sprintf("    if (%s.page) {\n", paramName))
			for _, pp := range pageParams {
				camelPP := toCamelCase(pp.Name)
				buf.WriteString(fmt.Sprintf("      if (%s.page.%s !== undefined && %s.page.%s !== null) {\n", paramName, camelPP, paramName, camelPP))
				buf.WriteString(fmt.Sprintf("        queryString.append('%s', String(%s.page.%s));\n", pp.Name, paramName, camelPP))
				buf.WriteString("      }\n")
			}
			buf.WriteString("    }\n")
		}

		if len(includeParams) > 0 {
			buf.WriteString(fmt.Sprintf("    if (%s.include) {\n", paramName))
			buf.WriteString(fmt.Sprintf("      queryString.append('include', %s.include.join(','));\n", paramName))
			buf.WriteString("    }\n")
		}

		buf.WriteString("    const finalUrl = queryString.toString() ? `${url}?${queryString.toString()}` : url;\n")
	} else {
		buf.WriteString("    const finalUrl = url;\n")
	}

	// Make the HTTP request
	buf.WriteString("    const response = await fetch(finalUrl, options);\n")
	buf.WriteString("    if (!response.ok) {\n")
	buf.WriteString("      // Handle errors appropriately\n")
	buf.WriteString("      throw new Error(`Request failed with status ${response.status}`);\n")
	buf.WriteString("    }\n")

	// Handle No Content responses (e.g., 204 No Content)
	if responseType == "any" || responseType == "void" {
		buf.WriteString("    return;\n")
	} else {
		buf.WriteString(fmt.Sprintf("    const data = await response.json();\n"))
		buf.WriteString("    // Transform keys to camelCase and recursively convert nested objects\n")
		buf.WriteString("    return toClientType(data);\n")
	}

	buf.WriteString("  }\n")

	return buf.String()
}

// parseBracketParam splits a parameter name like "filter[id]" into "filter" and "id"
func parseBracketParam(param string) (parent, key string) {
	re := regexp.MustCompile(`(\w+)\[(\w+)\]`)
	matches := re.FindStringSubmatch(param)
	if len(matches) == 3 {
		return matches[1], matches[2]
	}
	return "", ""
}

// extractPathParams identifies path parameters in the URL
func extractPathParams(path string) []string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)
	params := []string{}
	for _, match := range matches {
		if len(match) > 1 {
			params = append(params, match[1])
		}
	}
	return params
}

// getRefName extracts the reference name from a $ref string
func getRefName(ref string) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

// toCamelCase converts snake_case or any_case to camelCase
// toCamelCase converts a string to camelCase
func toCamelCase(input string) string {
	if input == "" {
		return ""
	}

	// remove leading _ if exists
	if input[0] == '_' {
		input = input[1:]
	}

	parts := strings.Split(input, "_")
	for i, part := range parts {
		if i == 0 {
			// Ensure the first part is lowercase
			parts[i] = strings.ToLower(part)
		} else {
			// Title case for subsequent parts
			parts[i] = strings.Title(part)
		}
	}

	return strings.Join(parts, "")
}

// toPascalCase converts snake_case or any_case to PascalCase
func toPascalCase(input string) string {
	parts := strings.Split(input, "_")
	for i, part := range parts {
		// Handle acronyms like "UUID" or "ID"
		if isAllUpper(part) && len(part) > 1 {
			parts[i] = part
		} else {
			parts[i] = strings.Title(part)
		}
	}
	return strings.Join(parts, "")
}

// isAllUpper checks if a string is all uppercase
func isAllUpper(s string) bool {
	for _, r := range s {
		if !unicode.IsUpper(r) && unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// isPrimitiveType checks if a TypeScript type is primitive
func isPrimitiveType(tsType string) bool {
	primitiveTypes := map[string]struct{}{
		"string":  {},
		"number":  {},
		"boolean": {},
		"null":    {},
		"any":     {},
		"any[]":   {},
		"void":    {},
	}
	_, exists := primitiveTypes[tsType]
	return exists
}

// sortStrings sorts a slice of strings alphabetically
func sortStrings(s []string) []string {
	sort.Strings(s)
	return s
}

// stripPageParams removes 'page[' and ']' from a parameter name
// e.g., 'page[number]' -> 'number'
func stripPageParams(p string) string {
	return strings.TrimSuffix(strings.TrimPrefix(p, "page["), "]")
}

// Parameter represents a simplified parameter structure
type Parameter struct {
	Name        string
	In          string
	Description string
	Schema      *openapi3.SchemaRef
	Required    bool
}
