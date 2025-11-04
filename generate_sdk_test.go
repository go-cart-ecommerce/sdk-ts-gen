package main

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

func TestDateRangeFilterGeneration(t *testing.T) {
	inputPath := filepath.Join("testdata", "date_range_input.yaml")
	goldenPath := filepath.Join("testdata", "date_range_golden.ts.golden")

	openAPISpec, err := ioutil.ReadFile(inputPath)
	assert.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(openAPISpec)
	assert.NoError(t, err)

	sdkCode := generateSDK(doc, []TypeDefinition{}, []ParamDefinition{})
	sdkString := string(sdkCode)

	golden, err := ioutil.ReadFile(goldenPath)
	assert.NoError(t, err)
	goldenString := string(golden)

	assert.Equal(t, goldenString, sdkString)
}

func TestBinaryResponseGeneration(t *testing.T) {
	// Create OpenAPI spec with binary response
	openAPISpec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /files/{id}/download:
    get:
      operationId: downloadFile
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: string
      responses:
        '200':
          description: File download
          content:
            application/pdf:
              schema:
                type: string
                format: binary
`

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(openAPISpec))
	assert.NoError(t, err)

	// Generate SDK
	sdkCode := generateSDK(doc, []TypeDefinition{}, []ParamDefinition{})
	sdkString := string(sdkCode)

	// Test that binary response handling is included
	assert.Contains(t, sdkString, "Blob")
	assert.Contains(t, sdkString, "response.blob()")
	assert.Contains(t, sdkString, "return blob;")
}

func TestMixedResponseTypes(t *testing.T) {
	// Create OpenAPI spec with both JSON and binary responses
	openAPISpec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          description: Success
          content:
            application/json:
              schema:
                type: object
  /files/{id}/download:
    get:
      operationId: downloadFile
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: string
      responses:
        '200':
          description: File download
          content:
            application/pdf:
              schema:
                type: string
                format: binary
`

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(openAPISpec))
	assert.NoError(t, err)

	// Generate SDK
	sdkCode := generateSDK(doc, []TypeDefinition{}, []ParamDefinition{})
	sdkString := string(sdkCode)

	// Test that both response types are handled
	assert.Contains(t, sdkString, "response.json()")
	assert.Contains(t, sdkString, "response.blob()")
	assert.Contains(t, sdkString, "return toClientType(data);")
	assert.Contains(t, sdkString, "return blob;")
}

func TestDateRangeFilterQueryStringGeneration(t *testing.T) {
	// Load the test OpenAPI specification
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("testdata/date_range_input.yaml")
	if err != nil {
		t.Fatalf("Failed to load OpenAPI spec: %v", err)
	}

	// Generate TypeScript SDK code
	typeDefinitions := getTypeDefinitions(doc)
	paramDefinitions := getParamDefinitions(doc)
	sdkCode := generateSDK(doc, typeDefinitions, paramDefinitions)

	// Convert to string for easier testing
	generatedCode := string(sdkCode)

	// Test that DateRange query string generation is correctly implemented
	expectedPatterns := []string{
		`if (dateRange.gte)`,
		`if (dateRange.lte)`,
		`if (dateRange.gt)`,
		`if (dateRange.lt)`,
		`dateRange.gte.toISOString()`,
		`dateRange.lte.toISOString()`,
		`dateRange.gt.toISOString()`,
		`dateRange.lt.toISOString()`,
	}

	for _, pattern := range expectedPatterns {
		assert.Contains(t, generatedCode, pattern, "Generated SDK should contain DateRange query string logic")
	}
}

func TestSDKTypeExtraction(t *testing.T) {
	// Test that x-gocart-sdk-type extension is properly extracted
	openAPISpec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /items:
    get:
      operationId: listItems
      parameters:
        - in: query
          name: filter[created_at]
          schema:
            type: string
          x-gocart-sdk-type: DateRange
        - in: query
          name: filter[status]
          schema:
            type: string
          x-gocart-sdk-type: String
      responses:
        '200':
          description: Success
          content:
            application/json:
              schema:
                type: object
`

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(openAPISpec))
	assert.NoError(t, err)

	// Get method definitions
	methodDefinitions := getMethodDefinitions(doc)
	assert.Len(t, methodDefinitions, 1)

	methodDef := methodDefinitions[0]
	assert.Equal(t, "listItems", methodDef.Name)

	// Check that SDK types are extracted
	queryParams := methodDef.QueryParams["query"]
	assert.Len(t, queryParams, 2)

	// Check DateRange parameter
	assert.Equal(t, "filter[created_at]", queryParams[0].Name)
	assert.Equal(t, "DateRange", queryParams[0].SDKType)

	// Check regular parameter
	assert.Equal(t, "filter[status]", queryParams[1].Name)
	assert.Equal(t, "String", queryParams[1].SDKType)
}

func TestBinaryContentTypeDetection(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expected    bool
	}{
		{"PDF", "application/pdf", true},
		{"Image JPEG", "image/jpeg", true},
		{"Video MP4", "video/mp4", true},
		{"Audio MP3", "audio/mpeg", true},
		{"CSV", "text/csv", true},
		{"JSON", "application/json", false},
		{"Plain Text", "text/plain", false},
		{"HTML", "text/html", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBinaryContentType(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTMLResponseGeneration(t *testing.T) {
	// Create OpenAPI spec with HTML response
	openAPISpec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /campaigns/v1/campaigns/{id}/preview:
    get:
      operationId: previewCampaignHTML
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: string
            format: uuid
        - in: query
          name: filter[lang]
          schema:
            type: string
            enum:
              - en
              - sq
        - in: query
          name: filter[currency]
          schema:
            type: string
            enum:
              - ALL
              - EUR
              - USD
      responses:
        '200':
          description: Campaign HTML preview
          content:
            text/html:
              schema:
                type: string
                description: HTML content of the campaign email
`

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(openAPISpec))
	assert.NoError(t, err)

	// Generate SDK
	sdkCode := generateSDK(doc, []TypeDefinition{}, []ParamDefinition{})
	sdkString := string(sdkCode)

	// Test that HTML response handling is included
	assert.Contains(t, sdkString, "Promise<string>")
	assert.Contains(t, sdkString, "response.text()")
	assert.Contains(t, sdkString, "return html;")
	assert.Contains(t, sdkString, "// Handle HTML response")

	// Test that the method name is correct
	assert.Contains(t, sdkString, "previewCampaignHTML")

	// Test that HTML response uses text() instead of json() for success response
	// (Note: response.json() is still used for error handling, which is correct)
	htmlMethodStart := strings.Index(sdkString, "previewCampaignHTML")
	assert.Greater(t, htmlMethodStart, -1, "Should contain previewCampaignHTML method")

	// Find the HTML response handling section
	htmlMethodCode := sdkString[htmlMethodStart:]
	// Check that HTML response handling comes after error handling
	htmlResponseIndex := strings.Index(htmlMethodCode, "// Handle HTML response")
	errorHandlingIndex := strings.Index(htmlMethodCode, "if (!response.ok)")

	assert.Greater(t, htmlResponseIndex, errorHandlingIndex, "HTML response handling should come after error handling")
	assert.Contains(t, htmlMethodCode[htmlResponseIndex:], "response.text()", "HTML method should use response.text() for success")
	assert.Contains(t, htmlMethodCode[htmlResponseIndex:], "return html;", "HTML method should return html")
}

func TestHTMLResponseTypeDetection(t *testing.T) {
	// Create OpenAPI spec with HTML response
	openAPISpec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /preview:
    get:
      operationId: getPreview
      responses:
        '200':
          description: HTML preview
          content:
            text/html:
              schema:
                type: string
`

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(openAPISpec))
	assert.NoError(t, err)

	// Get method definitions
	methodDefinitions := getMethodDefinitions(doc)
	assert.Len(t, methodDefinitions, 1)

	methodDef := methodDefinitions[0]
	assert.Equal(t, "getPreview", methodDef.Name)
	assert.Equal(t, "string", methodDef.ResponseType)
	assert.Equal(t, "text/html", methodDef.ResponseContentType)
}

func TestMixedResponseTypesWithHTML(t *testing.T) {
	// Create OpenAPI spec with JSON, HTML, and binary responses
	openAPISpec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          description: Success
          content:
            application/json:
              schema:
                type: object
  /campaigns/{id}/preview:
    get:
      operationId: previewCampaignHTML
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: string
      responses:
        '200':
          description: HTML preview
          content:
            text/html:
              schema:
                type: string
  /files/{id}/download:
    get:
      operationId: downloadFile
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: string
      responses:
        '200':
          description: File download
          content:
            application/pdf:
              schema:
                type: string
                format: binary
`

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(openAPISpec))
	assert.NoError(t, err)

	// Generate SDK
	sdkCode := generateSDK(doc, []TypeDefinition{}, []ParamDefinition{})
	sdkString := string(sdkCode)

	// Test that all response types are handled
	assert.Contains(t, sdkString, "response.json()")
	assert.Contains(t, sdkString, "response.text()")
	assert.Contains(t, sdkString, "response.blob()")
	assert.Contains(t, sdkString, "return toClientType(data);")
	assert.Contains(t, sdkString, "return html;")
	assert.Contains(t, sdkString, "return blob;")

	// Verify HTML response handling is correct
	// Check that previewCampaignHTML method exists and has correct signature
	assert.Contains(t, sdkString, "previewCampaignHTML")
	assert.Contains(t, sdkString, "Promise<string>")

	// Find the HTML response handling section relative to the method
	htmlMethodIndex := strings.Index(sdkString, "previewCampaignHTML")
	assert.Greater(t, htmlMethodIndex, -1, "Should contain previewCampaignHTML method")

	// Get a large chunk after the method declaration to include the full method body
	methodBodyStart := htmlMethodIndex
	// Look for the HTML response handling comment after the method starts
	htmlResponseCommentIndex := strings.Index(sdkString[methodBodyStart:], "// Handle HTML response")
	assert.Greater(t, htmlResponseCommentIndex, -1, "Should contain HTML response handling comment")

	// Verify the HTML response handling section
	htmlResponseSection := sdkString[methodBodyStart+htmlResponseCommentIndex:]
	assert.Contains(t, htmlResponseSection, "response.text()", "HTML response section should use response.text()")
	assert.Contains(t, htmlResponseSection, "return html;", "HTML response section should return html")
}

func TestDetermineResponseTypeWithHTML(t *testing.T) {
	// Create OpenAPI spec with HTML response
	openAPISpec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /preview:
    get:
      operationId: getPreview
      responses:
        '200':
          description: HTML preview
          content:
            text/html:
              schema:
                type: string
                description: HTML content
`

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(openAPISpec))
	assert.NoError(t, err)

	// Get the operation
	pathItem := doc.Paths.Find("/preview")
	assert.NotNil(t, pathItem)
	assert.NotNil(t, pathItem.Get)

	operation := pathItem.Get

	// Test determineResponseType
	responseType, responseContentType, schemaRef := determineResponseType(operation)

	assert.Equal(t, "string", responseType)
	assert.Equal(t, "text/html", responseContentType)
	assert.NotNil(t, schemaRef)
}

func TestHTMLResponsePriority(t *testing.T) {
	// Test that HTML response is detected even when multiple content types are present
	openAPISpec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /preview:
    get:
      operationId: getPreview
      responses:
        '200':
          description: HTML preview
          content:
            text/html:
              schema:
                type: string
            application/json:
              schema:
                type: object
`

	// Parse the OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(openAPISpec))
	assert.NoError(t, err)

	// Get method definitions
	methodDefinitions := getMethodDefinitions(doc)
	assert.Len(t, methodDefinitions, 1)

	methodDef := methodDefinitions[0]
	// HTML should be detected before JSON (based on the order in determineResponseType)
	// Since HTML is checked before JSON in the code, it should return HTML
	assert.Equal(t, "string", methodDef.ResponseType)
	assert.Equal(t, "text/html", methodDef.ResponseContentType)
}

func TestPrimitiveTypeDetection(t *testing.T) {
	tests := []struct {
		name     string
		tsType   string
		expected bool
	}{
		{"string", "string", true},
		{"number", "number", true},
		{"boolean", "boolean", true},
		{"Blob", "Blob", true},
		{"DateRange", "DateRange", true},
		{"CustomType", "CustomType", false},
		{"User", "User", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrimitiveType(tt.tsType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBracketParamParsing(t *testing.T) {
	tests := []struct {
		name     string
		param    string
		expected struct {
			parent string
			key    string
		}
	}{
		{
			name:  "filter[created_at]",
			param: "filter[created_at]",
			expected: struct {
				parent string
				key    string
			}{
				parent: "filter",
				key:    "created_at",
			},
		},
		{
			name:  "page[number]",
			param: "page[number]",
			expected: struct {
				parent string
				key    string
			}{
				parent: "page",
				key:    "number",
			},
		},
		{
			name:  "invalid format",
			param: "invalid-format",
			expected: struct {
				parent string
				key    string
			}{
				parent: "",
				key:    "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent, key := parseBracketParam(tt.param)
			assert.Equal(t, tt.expected.parent, parent)
			assert.Equal(t, tt.expected.key, key)
		})
	}
}

func TestDateRangeParameterGeneration(t *testing.T) {
	inputPath := filepath.Join("testdata", "date_range_input.yaml")

	openAPISpec, err := ioutil.ReadFile(inputPath)
	assert.NoError(t, err)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(openAPISpec)
	assert.NoError(t, err)

	// Generate parameters
	paramDefs := getParamDefinitions(doc)
	paramsCode := generateParams(doc, paramDefs)
	paramsString := string(paramsCode)

	// Test that DateRange type is defined
	assert.Contains(t, paramsString, "export interface DateRange {")
	assert.Contains(t, paramsString, "gte?: Date;")
	assert.Contains(t, paramsString, "lte?: Date;")
	assert.Contains(t, paramsString, "gt?: Date;")
	assert.Contains(t, paramsString, "lt?: Date;")

	// Test that filter parameters use DateRange type
	assert.Contains(t, paramsString, "createdAt?: DateRange;")
	assert.Contains(t, paramsString, "updatedAt?: DateRange;")

	// Test that regular string parameters don't use DateRange
	assert.Contains(t, paramsString, "name?: string;")
}

func TestComprehensiveFiltersGeneration(t *testing.T) {
	// Load the comprehensive test OpenAPI specification
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("testdata/comprehensive_filters.yaml")
	if err != nil {
		t.Fatalf("Failed to load OpenAPI spec: %v", err)
	}

	// Generate TypeScript SDK code
	typeDefinitions := getTypeDefinitions(doc)
	paramDefinitions := getParamDefinitions(doc)
	sdkCode := generateSDK(doc, typeDefinitions, paramDefinitions)

	// Convert to string for easier testing
	generatedCode := string(sdkCode)

	// Test that all filter types are handled correctly
	tests := []struct {
		name     string
		contains string
	}{
		// Range type filters should use the range query builder
		{
			name:     "DateRange filter",
			contains: `const dateRange = params.filter["clientRegistrationDate"];`,
		},
		{
			name:     "NumberRange filter - order count",
			contains: `const numberRange = params.filter["orderCount"];`,
		},
		{
			name:     "CurrencyRange filter",
			contains: `const currencyRange = params.filter["orderAmount"];`,
		},
		{
			name:     "NumberRange filter - cart article count",
			contains: `const numberRange = params.filter["cartArticleCount"];`,
		},
		// Non-range filters should use formatFilterValue function
		{
			name:     "String filter - email",
			contains: `queryString.append('filter[email]', this.formatFilterValue(value));`,
		},
		{
			name:     "String filter - role",
			contains: `queryString.append('filter[role]', this.formatFilterValue(value));`,
		},
		{
			name:     "Boolean filter - is_blocked",
			contains: `queryString.append('filter[is_blocked]', this.formatFilterValue(value));`,
		},
		{
			name:     "Boolean filter - is_email_verified",
			contains: `queryString.append('filter[is_email_verified]', this.formatFilterValue(value));`,
		},
		{
			name:     "UUID filter - group_id",
			contains: `queryString.append('filter[group_id]', this.formatFilterValue(value));`,
		},
		// Check that range operators are generated correctly
		{
			name:     "DateRange gte operator",
			contains: "if (dateRange.gte) { queryString.append('filter[client_registration_date]', `>=${dateRange.gte.toISOString()}`); }",
		},
		{
			name:     "NumberRange min/max range",
			contains: "if (range.min !== undefined || range.max !== undefined) {",
		},
		{
			name:     "CurrencyRange with currency prefix",
			contains: "queryString.append('filter[order_amount]', `${range.currency}:${valueStr}`);",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(generatedCode, test.contains) {
				t.Errorf("Generated code should contain: %s", test.contains)
				// Print relevant section for debugging
				lines := strings.Split(generatedCode, "\n")
				for i, line := range lines {
					if strings.Contains(line, "filter") {
						start := max(0, i-2)
						end := min(len(lines), i+3)
						t.Logf("Context around line %d:\n%s", i+1, strings.Join(lines[start:end], "\n"))
						break
					}
				}
			}
		})
	}

	// Also test that param types are generated correctly
	paramCode := generateParams(doc, paramDefinitions)
	paramCodeStr := string(paramCode)

	paramTests := []struct {
		name     string
		contains string
	}{
		{
			name:     "DateRange interface definition",
			contains: "export interface DateRange {",
		},
		{
			name:     "NumberRange interface definition",
			contains: "export interface NumberRange {",
		},
		{
			name:     "CurrencyRange interface definition",
			contains: "export interface CurrencyRange {",
		},
		{
			name:     "DateRange filter type",
			contains: "clientRegistrationDate?: DateRange;",
		},
		{
			name:     "NumberRange filter type",
			contains: "orderCount?: NumberRange;",
		},
		{
			name:     "CurrencyRange filter type",
			contains: "orderAmount?: CurrencyRange;",
		},
		{
			name:     "String filter type",
			contains: "email?: string;",
		},
		{
			name:     "Boolean filter type",
			contains: "isBlocked?: boolean;",
		},
	}

	for _, test := range paramTests {
		t.Run("Params_"+test.name, func(t *testing.T) {
			if !strings.Contains(paramCodeStr, test.contains) {
				t.Errorf("Generated params should contain: %s", test.contains)
				t.Logf("Generated params code:\n%s", paramCodeStr)
			}
		})
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
