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

// QueryParameter represents a simplified parameter structure
type QueryParameter struct {
	Name        string
	In          string
	Description string
	Schema      *openapi3.SchemaRef
	Required    bool
}

type MethodDefinition struct {
	Name            string
	Arguments       MethodArgumentDefinitions
	ResponseType    string
	HTTPMethod      string
	Path            string
	QueryParams     map[string][]QueryParameter
	OperationRef    *openapi3.Operation
	ResponseTypeRef *openapi3.SchemaRef
}

type MethodDefinitions []MethodDefinition

func (m MethodDefinitions) HasMethod(name string) bool {
	for _, p := range m {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (m MethodDefinitions) GetMethod(name string) (MethodDefinition, bool) {
	for _, p := range m {
		if p.Name == name {
			return p, true
		}
	}
	return MethodDefinition{}, false
}

func (m MethodDefinitions) UseType(typeName string) bool {
	for _, p := range m {
		for _, arg := range p.Arguments {
			if arg.Type.Name == typeName {
				return true
			}
		}

		if p.ResponseType == typeName {
			return true
		}
	}

	return false
}

func (m MethodDefinitions) Sort() {
	sort.Slice(m, func(i, j int) bool {
		return m[i].Name < m[j].Name
	})
}

type MethodArgumentDefinition struct {
	Name string
	Type TypeDefinition
}

type MethodArgumentDefinitions []MethodArgumentDefinition

func (m MethodArgumentDefinitions) HasParam(name string) bool {
	for _, p := range m {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (m MethodArgumentDefinitions) GetPayloadParam() (MethodArgumentDefinition, bool) {
	for _, p := range m {
		if p.Name == "req" {
			return p, true
		}
	}
	return MethodArgumentDefinition{}, false
}

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

func getMethodDefinitions(doc *openapi3.T) MethodDefinitions {
	var methodDefinitions MethodDefinitions

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

			// Determine method name
			methodName := generateMethodName(operation, method, path)

			// Determine if the method has a request body
			// isReferencingResponseType := false
			var requestType string
			if operation.RequestBody != nil && operation.RequestBody.Value != nil {
				content := operation.RequestBody.Value.Content
				if appJSON, ok := content["application/json"]; ok && appJSON.Schema != nil {
					if appJSON.Schema.Ref != "" {
						requestType = toPascalCase(getRefName(appJSON.Schema.Ref))
						// isReferencingResponseType = true
					} else {
						// Handle inline schemas or other types
						requestType = toPascalCase(methodName) + "Request"
					}

				}

				if appFormData, ok := content["multipart/form-data"]; ok && appFormData.Schema != nil {
					if appFormData.Schema.Ref != "" {
						requestType = toPascalCase(getRefName(appFormData.Schema.Ref))
						// isReferencingResponseType = true
					} else {
						// Handle inline schemas or other types
						requestType = toPascalCase(methodName) + "Request"
					}
				}
			}

			// Extract parameters
			queryParams := extractParameters(operation)

			var methodArgumentList MethodArgumentDefinitions

			// Determine parameter type and name based on HTTP method
			switch strings.ToUpper(method) {
			case "POST":
				methodArgumentList = append(methodArgumentList, MethodArgumentDefinition{
					Name: "req",
					Type: TypeDefinition{
						Name: requestType,
					},
				})
			case "PATCH", "PUT":
				// Assume PATCH methods have an 'id' parameter and a request body
				methodArgumentList = append(methodArgumentList, MethodArgumentDefinition{
					Name: "id",
					Type: TypeDefinition{
						Name: "string",
					},
				}, MethodArgumentDefinition{
					Name: "req",
					Type: TypeDefinition{
						Name: requestType,
					},
				})

			case "DELETE":
				// Assume DELETE methods only have a single string parameter
				methodArgumentList = append(methodArgumentList, MethodArgumentDefinition{
					Name: "id",
					Type: TypeDefinition{
						Name: "string",
					},
				})
			case "GET":
				// check if it is getOne or getAll
				if !strings.Contains(methodName, "list") {
					methodArgumentList = append(methodArgumentList, MethodArgumentDefinition{
						Name: "id",
						Type: TypeDefinition{
							Name: "string",
						},
					})
				}
				paramTypeName := toPascalCase(methodName) + "Params"
				methodArgumentList = append(methodArgumentList, MethodArgumentDefinition{
					Name: "params",
					Type: TypeDefinition{
						Name:     paramTypeName,
						Optional: true,
					},
				})
			default:
				fmt.Println("unsupported method:", method)
			}

			// Determine response type
			responseType, ResponseTypeRef := determineResponseType(operation)

			methodDefinitions = append(methodDefinitions, MethodDefinition{
				Name:            methodName,
				HTTPMethod:      method,
				Path:            path,
				Arguments:       methodArgumentList,
				ResponseType:    responseType,
				QueryParams:     queryParams,
				OperationRef:    operation,
				ResponseTypeRef: ResponseTypeRef,
			})
		}
	}

	methodDefinitions.Sort()

	return methodDefinitions
}

func generateSDK(doc *openapi3.T, typeDefinitions []TypeDefinition, paramDefinitions []ParamDefinition) []byte {
	methodDefinitions := getMethodDefinitions(doc)

	// Generate import statements with collected types
	importTypes := []string{}
	for _, m := range typeDefinitions {
		if !isPrimitiveType(m.Name) && !isArrayType(m.Name) && methodDefinitions.UseType(m.Name) {
			importTypes = append(importTypes, m.Name)
		}
	}
	sort.Strings(importTypes)

	importParams := []string{}
	for _, m := range paramDefinitions {
		if !isPrimitiveType(m.Name) && !isArrayType(m.Name) && methodDefinitions.UseType(m.Name) {
			importParams = append(importParams, m.Name)
		}
	}
	sort.Strings(importParams)

	// Prepare to collect all TypeScript methods and types
	var tsBuffer bytes.Buffer
	tsBuffer.WriteString("// Auto-generated TypeScript SDK\n")
	tsBuffer.WriteString("// Do not modify manually.\n\n")

	// Generate import statement for types.ts
	if len(importTypes) > 0 {
		tsBuffer.WriteString("import {\n")
		for _, tsType := range importTypes {
			tsBuffer.WriteString(fmt.Sprintf("  %s,\n", tsType))
		}
		tsBuffer.WriteString("  APIError,\n")
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
		tsBuffer.WriteString("} from './params';\n\n")
	}

	tsBuffer.WriteString("import { InMemoryContext } from './context';\n")
	tsBuffer.WriteString("import { ApiError } from './error';\n")
	tsBuffer.WriteString("import { toApiType, toClientType } from './utils';\n")
	tsBuffer.WriteString("import { RequestInterceptor, ResponseInterceptor, InterceptorManager } from './interceptors';\n\n")
	tsBuffer.WriteString("const SDK_VERSION = 'unset';\n\n")

	// Start GoCartSDK class
	tsBuffer.WriteString("export class GoCartSDK {\n")
	tsBuffer.WriteString("  private baseUrl: string;\n\n")
	tsBuffer.WriteString("  public context: InMemoryContext;\n")
	tsBuffer.WriteString("  public interceptors: {\n")
	tsBuffer.WriteString("    request: InterceptorManager<RequestInterceptor>;\n")
	tsBuffer.WriteString("    response: InterceptorManager<ResponseInterceptor>;\n")
	tsBuffer.WriteString("  };\n\n")

	tsBuffer.WriteString("  constructor(baseUrl: string = 'https://api.orbita.al') {\n")
	tsBuffer.WriteString("    this.baseUrl = baseUrl;\n")
	tsBuffer.WriteString("    this.context = new InMemoryContext();\n")
	tsBuffer.WriteString("    this.interceptors = {\n")
	tsBuffer.WriteString("      request: new InterceptorManager<RequestInterceptor>(),\n")
	tsBuffer.WriteString("      response: new InterceptorManager<ResponseInterceptor>()\n")
	tsBuffer.WriteString("    };\n")
	tsBuffer.WriteString("  }\n\n")

	for _, m := range methodDefinitions {
		mthodCode := generateMethod(doc, m)
		tsBuffer.WriteString(mthodCode)
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
func extractParameters(operation *openapi3.Operation) map[string][]QueryParameter {
	groupedParams := make(map[string][]QueryParameter)

	for _, paramRef := range operation.Parameters {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		param := paramRef.Value
		p := QueryParameter{
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
			p := QueryParameter{
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
func determineResponseType(operation *openapi3.Operation) (string, *openapi3.SchemaRef) {
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
						return toPascalCase(getRefName(schema.Ref)), schema
					} else {
						// Handle inline schemas or other types
						return resolveInlineType(schema), schema
					}
				}
			}
		}
	}

	// Fallback to 'any' if no suitable response found
	return "any", nil
}

// resolveInlineType resolves TypeScript types from inline OpenAPI schemas
func resolveInlineType(schema *openapi3.SchemaRef) string {
	if schema.Value.Type != nil {
		if len(*schema.Value.Type) == 1 {
			openType := strings.ToLower((*schema.Value.Type)[0])
			if openType == "array" {
				// Handle array types
				if schema.Value.Items != nil {
					itemType := ""
					if schema.Value.Items.Ref != "" {
						itemType = toPascalCase(getRefName(schema.Value.Items.Ref))
					} else {
						itemType = resolveInlineType(schema.Value.Items)
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
					} else {
						itemType = resolveInlineType(schema.Value.Items)
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

func getEmbeddedKeysFromSchema(schema *openapi3.SchemaRef) []string {
	var embeddedObjects []string

	if embeddedSchema, exists := schema.Value.Properties["_embedded"]; exists && embeddedSchema.Value != nil {
		// Iterate over properties within `_embedded`
		for propName, propSchema := range embeddedSchema.Value.Properties {
			// Check if the property is an object (excluding arrays and primitives)
			if propSchema.Value != nil && propSchema.Value.Type.Is("object") && propSchema.Value.Items == nil {
				embeddedObjects = append(embeddedObjects, toCamelCase(propName))
			}

			// array of objects
			if propSchema.Value != nil && propSchema.Value.Type.Is("array") && propSchema.Value.Items != nil {
				if propSchema.Value.Items.Ref != "" {
					embeddedObjects = append(embeddedObjects, toCamelCase(propName))
				}
			}
		}
	}

	return embeddedObjects
}

// generateMethod creates a TypeScript method within the GoCartSDK class
func generateMethod(doc *openapi3.T, methodDefinition MethodDefinition) string {
	var buf bytes.Buffer

	// Generate JSDoc comments
	buf.WriteString("  /**\n")
	buf.WriteString(fmt.Sprintf("   * %s\n", methodDefinition.Name))
	for _, p := range methodDefinition.Arguments {
		buf.WriteString(fmt.Sprintf("   * @param %s %s\n", p.Name, p.Type.Name))
	}
	buf.WriteString(fmt.Sprintf("   * @returns Promise<%s>\n", methodDefinition.ResponseType))
	buf.WriteString("   */\n")

	// Generate method signature
	paramsSignature := []string{}
	for _, p := range methodDefinition.Arguments {
		if p.Type.Optional {
			paramsSignature = append(paramsSignature, fmt.Sprintf("%s: %s = {}", p.Name, p.Type.Name))
		} else {
			paramsSignature = append(paramsSignature, fmt.Sprintf("%s: %s", p.Name, p.Type.Name))
		}
	}
	buf.WriteString(fmt.Sprintf("  public async %s(%s): Promise<%s> {\n", methodDefinition.Name, strings.Join(paramsSignature, ", "), methodDefinition.ResponseType))

	// Construct URL with path parameters
	url := methodDefinition.Path
	pathParams := extractPathParams(url)
	if len(pathParams) > 0 {
		for _, p := range pathParams {
			camelParam := toCamelCase(p)
			url = strings.ReplaceAll(url, "{"+p+"}", fmt.Sprintf("${%s}", camelParam))
		}
		buf.WriteString(fmt.Sprintf("    const url = `${this.baseUrl}%s`;\n", url))
	} else {
		buf.WriteString(fmt.Sprintf("    const url = `${this.baseUrl}%s`;\n", url))
	}

	if methodDefinition.HTTPMethod == "POST" || methodDefinition.HTTPMethod == "PUT" || methodDefinition.HTTPMethod == "PATCH" {
		if methodDefinition.OperationRef.RequestBody != nil && methodDefinition.OperationRef.RequestBody.Value != nil && methodDefinition.OperationRef.RequestBody.Value.Content["application/json"] != nil {

			// create an array with keys of embedded objects in the response schema
			var embeddedObjects []string
			requestSchema := methodDefinition.OperationRef.RequestBody.Value.Content["application/json"].Schema

			if requestSchema.Value.Type.Is("object") {
				if methodDefinition.ResponseTypeRef != nil && methodDefinition.ResponseTypeRef.Value != nil {
					schemaRef := methodDefinition.ResponseTypeRef
					// Look for the `_embedded` property
					embeddedObjects = getEmbeddedKeysFromSchema(schemaRef)
				}
			} else if requestSchema.Value.Type.Is("array") {
				// Handle array of objects
				if requestSchema.Value.Items != nil && requestSchema.Value.Items.Ref != "" {
					// resolve the ref
					itemSchema, _ := resolveSchemaRef(requestSchema.Value.Items, doc)
					if itemSchema.Value.Type.Is("object") {
						embeddedObjects = getEmbeddedKeysFromSchema(requestSchema.Value.Items)
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
			if param, ok := methodDefinition.Arguments.GetPayloadParam(); ok {
				buf.WriteString(fmt.Sprintf("		const body = toApiType(%s, embeddedObjects);\n", param.Name))
			}

			// Initialize options for fetch
			buf.WriteString("    let options: RequestInit = {\n")
			buf.WriteString(fmt.Sprintf("      method: '%s',\n", strings.ToUpper(methodDefinition.HTTPMethod)))
			buf.WriteString("      headers: {\n")
			buf.WriteString("        'Content-Type': 'application/json',\n")
			buf.WriteString("        'x-gocart-sdk-version': SDK_VERSION,\n")
			buf.WriteString("        // Add other headers like authentication here\n")
			buf.WriteString("      },\n")

			if _, ok := methodDefinition.Arguments.GetPayloadParam(); ok {
				buf.WriteString("      body: JSON.stringify(body),\n")
			}

			buf.WriteString("    };\n")
		}

		if methodDefinition.OperationRef.RequestBody != nil && methodDefinition.OperationRef.RequestBody.Value != nil && methodDefinition.OperationRef.RequestBody.Value.Content["multipart/form-data"] != nil {
			buf.Write([]byte("    // This is a multipart/form-data request\n"))
			buf.Write([]byte("    // We need to create a FormData object and append fields to it\n"))
			buf.Write([]byte("    let formData = new FormData();\n"))
			buf.Write([]byte("    // Add form fields to formData\n"))
			// iterate over responseTypeRef properties, is it is an object, json.stringify, else append to formData
			for pName, pSchema := range methodDefinition.OperationRef.RequestBody.Value.Content["multipart/form-data"].Schema.Value.Properties {
				if pSchema.Value.Type.Is("object") {
					buf.WriteString(fmt.Sprintf("    if (req.%s) {\n", toCamelCase(pName)))
					buf.WriteString(fmt.Sprintf("      formData.append('%s', JSON.stringify(req.%s));\n", pName, toCamelCase(pName)))
					buf.WriteString("    }\n")
				} else {
					buf.WriteString(fmt.Sprintf("    if (req.%s !== undefined && req.%s !== null) {\n", toCamelCase(pName), toCamelCase(pName)))
					buf.WriteString(fmt.Sprintf("      formData.append('%s', req.%s);\n", pName, toCamelCase(pName)))
					buf.WriteString("    }\n")
				}
			}

			buf.WriteString("    // Configure the fetch options\n")
			buf.WriteString("    let options: RequestInit = {\n")
			buf.WriteString(fmt.Sprintf("      method: '%s',\n", strings.ToUpper(methodDefinition.HTTPMethod)))
			buf.WriteString("      headers: {\n")
			buf.WriteString("        // Do not set 'Content-Type' header when sending FormData\n")
			buf.WriteString("        // The browser will automatically set it, including the boundary\n")
			buf.WriteString("        'Accept': 'application/json',\n")
			buf.WriteString("        'x-gocart-sdk-version': SDK_VERSION,\n")
			buf.WriteString("        // Add other headers like authentication here\n")
			buf.WriteString("      },\n")
			buf.WriteString("      body: formData,\n")
			buf.WriteString("    };\n")

		}
	} else {
		// Initialize options for fetch
		buf.WriteString("    let options: RequestInit = {\n")
		buf.WriteString(fmt.Sprintf("      method: '%s',\n", strings.ToUpper(methodDefinition.HTTPMethod)))
		buf.WriteString("      headers: {\n")
		buf.WriteString("        'Content-Type': 'application/json',\n")
		buf.WriteString("        'x-gocart-sdk-version': SDK_VERSION,\n")
		buf.WriteString("        // Add other headers like authentication here\n")
		buf.WriteString("      },\n")
		buf.WriteString("    };\n")
	}

	if strings.HasPrefix(methodDefinition.Name, "list") {
		buf.WriteString("    if (params.totalCount) {\n")
		buf.WriteString("      options.headers = {\n")
		buf.WriteString("        ...options.headers,\n")
		buf.WriteString("        'Collection-Total': 'include'\n")
		buf.WriteString("      }\n")
		buf.WriteString("    }\n")
	}

	// Handle query parameters (only for methods that can have query params, typically GET, DELETE)
	// Assuming that methods with 'params' can have query parameters
	if methodDefinition.Arguments.HasParam("params") && methodDefinition.QueryParams["query"] != nil {
		paramName := "params"

		buf.WriteString("    const queryString = new URLSearchParams();\n")

		// Separate filter, sort, and page parameters
		var filterParams, sortParams, pageParams, includeParams []QueryParameter
		for _, qp := range methodDefinition.QueryParams["query"] {
			switch {
			case strings.HasPrefix(qp.Name, "filter["):
				filterParams = append(filterParams, QueryParameter{
					Name:        toCamelCase(qp.Name),
					In:          "query",
					Description: qp.Description,
					Schema:      qp.Schema,
					Required:    qp.Required,
				})
			case qp.Name == "sort":
				sortParams = append(sortParams, qp)
			case strings.HasPrefix(qp.Name, "page["):
				pageParams = append(pageParams, QueryParameter{
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
					buf.WriteString(fmt.Sprintf("        queryString.append('%slesh', String(%s.%s));\n", fp.Name, paramName, camelQP))
					buf.WriteString("      }\n")
					continue
				}

				camelParent := toCamelCase(parent)
				// camelKey := toCamelCase(key) // Not used in this context
				buf.WriteString(fmt.Sprintf("      if (%s.%s[\"%s\"] !== undefined && %s.%s[\"%s\"] !== null) {\n", paramName, camelParent, key, paramName, camelParent, key))
				buf.WriteString(fmt.Sprintf("        queryString.append('%s[%s]', String(%s.%s[\"%s\"]));\n", camelParent, toSnakeCase(key), paramName, camelParent, key))
				buf.WriteString("      }\n")
			}
			buf.WriteString("    }\n")
		}

		// Handle sort parameters
		if len(sortParams) > 0 {
			for _, sp := range sortParams {
				camelSP := toCamelCase(sp.Name)
				buf.WriteString(fmt.Sprintf("    if (%s.%s !== undefined && %s.%s !== null) {\n", paramName, camelSP, paramName, camelSP))
				// join array and replace camelCase with snake_case
				buf.WriteString(fmt.Sprintf("      queryString.append('%s', %s.%s.map((v)=> v.replace(/([A-Z])/g, '_$1').toLowerCase()).join(','));\n", sp.Name, paramName, camelSP))
				buf.WriteString("    }\n")
			}
		}

		// Handle pagination parameters
		if len(pageParams) > 0 {
			buf.WriteString(fmt.Sprintf("    if (%s.page) {\n", paramName))
			for _, pp := range pageParams {
				camelPP := toCamelCase(pp.Name)
				buf.WriteString(fmt.Sprintf("      if (%s.page.%s !== undefined && %s.page.%s !== null) {\n", paramName, camelPP, paramName, camelPP))
				buf.WriteString(fmt.Sprintf("        queryString.append('page[%s]', String(%s.page.%s));\n", pp.Name, paramName, camelPP))
				buf.WriteString("      }\n")
			}
			buf.WriteString("    }\n")
		}

		if len(includeParams) > 0 {
			buf.WriteString(fmt.Sprintf("    if (%s.include) {\n", paramName))
			buf.WriteString("      queryString.append('include', params.include.map((v) => {\n")
			buf.WriteString("        // First handle path segments with dots\n")
			buf.WriteString("        return v.split('.').map(segment => {\n")
			buf.WriteString("          // Convert camelCase or PascalCase to snake_case\n")
			buf.WriteString("          return segment.replace(/([a-z])([A-Z])/g, '$1_$2').toLowerCase();\n")
			buf.WriteString("        }).join('.');\n")
			buf.WriteString("      }).join(','));\n")
			buf.WriteString("    }\n")
		}

		buf.WriteString("    let finalUrl = queryString.toString() ? `${url}?${queryString.toString()}` : url;\n")
	} else {
		buf.WriteString("    let finalUrl = url;\n")
	}

	// Make the HTTP request
	buf.WriteString("    options = this.context.setHttpRequestHeaders(options);\n")
	buf.WriteString("    // Apply request interceptors\n")
	buf.WriteString("    for (const interceptor of this.interceptors.request.interceptors) {\n")
	buf.WriteString("      const result = await interceptor(options, finalUrl);\n")
	buf.WriteString("      if (result) {\n")
	buf.WriteString("        if (result.options) options = result.options;\n")
	buf.WriteString("        if (result.url) finalUrl = result.url;\n")
	buf.WriteString("      }\n")
	buf.WriteString("    }\n\n")
	buf.WriteString("    let response = await fetch(finalUrl, options);\n")
	buf.WriteString("    // Apply response interceptors\n")
	buf.WriteString("    for (const interceptor of this.interceptors.response.interceptors) {\n")
	buf.WriteString("      const result = await interceptor(response, options, finalUrl);\n")
	buf.WriteString("      if (result) {\n")
	buf.WriteString("        response = result;\n")
	buf.WriteString("      }\n")
	buf.WriteString("    }\n\n")
	buf.WriteString("    if (!response.ok) {\n")
	buf.WriteString("      const errMessage: APIError = await response.json();\n")
	buf.WriteString("      throw new ApiError(errMessage.code, errMessage.message, errMessage.fieldErrors);\n")
	buf.WriteString("    }\n")

	// Handle No Content responses (e.g., 204 No Content)
	if methodDefinition.ResponseType == "any" || methodDefinition.ResponseType == "void" {
		buf.WriteString("    return;\n")
	} else {
		buf.WriteString("    const data = await response.json();\n")
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

func toSnakeCase(input string) string {
	if input == "" {
		return ""
	}

	// remove leading _ if exists
	if input[0] == '_' {
		input = input[1:]
	}

	var output []rune
	for i, r := range input {
		if unicode.IsUpper(r) {
			if i > 0 {
				output = append(output, '_')
			}
			output = append(output, unicode.ToLower(r))
		} else {
			output = append(output, r)
		}
	}
	return string(output)
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

// stripPageParams removes 'page[' and ']' from a parameter name
// e.g., 'page[number]' -> 'number'
func stripPageParams(p string) string {
	return strings.TrimSuffix(strings.TrimPrefix(p, "page["), "]")
}
