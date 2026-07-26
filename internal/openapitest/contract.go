package openapitest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

const CanonicalDocumentPath = "docs/openapi.yaml"

var errInvalidContract = errors.New("openapi_contract_invalid")

type OperationKey struct {
	Method string
	Path   string
}

type SecurityScheme struct {
	Type string
	In   string
	Name string
}

type Contract struct {
	document map[string]any
}

func Load(path string) (*Contract, error) {
	documentBytes, readError := os.ReadFile(path)
	if readError != nil {
		return nil, fmt.Errorf("%w: read path=%s: %v", errInvalidContract, path, readError)
	}
	decoder := json.NewDecoder(bytes.NewReader(documentBytes))
	decoder.UseNumber()
	var document map[string]any
	if decodeError := decoder.Decode(&document); decodeError != nil {
		return nil, fmt.Errorf("%w: decode path=%s: %v", errInvalidContract, path, decodeError)
	}
	var trailingValue any
	if trailingError := decoder.Decode(&trailingValue); !errors.Is(trailingError, io.EOF) {
		if trailingError == nil {
			return nil, fmt.Errorf("%w: path=%s contains multiple documents", errInvalidContract, path)
		}
		return nil, fmt.Errorf("%w: trailing content path=%s: %v", errInvalidContract, path, trailingError)
	}
	if version, _ := document["openapi"].(string); version != "3.1.0" {
		return nil, fmt.Errorf("%w: path=%s openapi=%q want=3.1.0", errInvalidContract, path, version)
	}
	return &Contract{document: document}, nil
}

func (contract *Contract) ServerURLs() ([]string, error) {
	rawServers, ok := contract.document["servers"].([]any)
	if !ok {
		return nil, fmt.Errorf("%w: servers must be an array", errInvalidContract)
	}
	serverURLs := make([]string, 0, len(rawServers))
	for serverIndex, rawServer := range rawServers {
		server, objectError := contractObject(rawServer, fmt.Sprintf("servers[%d]", serverIndex))
		if objectError != nil {
			return nil, objectError
		}
		serverURL, ok := server["url"].(string)
		if !ok || strings.TrimSpace(serverURL) == "" {
			return nil, fmt.Errorf("%w: servers[%d].url must be a nonblank string", errInvalidContract, serverIndex)
		}
		serverURLs = append(serverURLs, serverURL)
	}
	return serverURLs, nil
}

func (contract *Contract) ProtocolMethods() (map[string]struct{}, error) {
	rawMethods, ok := contract.document["x-llm-proxy-protocol-methods"].([]any)
	if !ok {
		return nil, fmt.Errorf("%w: x-llm-proxy-protocol-methods must be an array", errInvalidContract)
	}
	methods := make(map[string]struct{}, len(rawMethods))
	for methodIndex, rawMethod := range rawMethods {
		method, ok := rawMethod.(string)
		if !ok || strings.TrimSpace(method) == "" {
			return nil, fmt.Errorf("%w: x-llm-proxy-protocol-methods[%d] must be a nonblank string", errInvalidContract, methodIndex)
		}
		methods[strings.ToUpper(method)] = struct{}{}
	}
	return methods, nil
}

func (contract *Contract) Operations() ([]OperationKey, error) {
	paths, objectError := contractObject(contract.document["paths"], "paths")
	if objectError != nil {
		return nil, objectError
	}
	operations := make([]OperationKey, 0)
	for path, rawPathItem := range paths {
		pathItem, pathItemError := contractObject(rawPathItem, "paths."+path)
		if pathItemError != nil {
			return nil, pathItemError
		}
		for method, rawOperation := range pathItem {
			normalizedMethod := strings.ToUpper(method)
			if !isHTTPMethod(normalizedMethod) {
				continue
			}
			if _, operationError := contractObject(rawOperation, "paths."+path+"."+method); operationError != nil {
				return nil, operationError
			}
			operations = append(operations, OperationKey{Method: normalizedMethod, Path: path})
		}
	}
	sort.Slice(operations, func(leftIndex int, rightIndex int) bool {
		if operations[leftIndex].Path == operations[rightIndex].Path {
			return operations[leftIndex].Method < operations[rightIndex].Method
		}
		return operations[leftIndex].Path < operations[rightIndex].Path
	})
	return operations, nil
}

func (contract *Contract) SecurityScheme(name string) (SecurityScheme, error) {
	components, componentsError := contractObject(contract.document["components"], "components")
	if componentsError != nil {
		return SecurityScheme{}, componentsError
	}
	securitySchemes, securitySchemesError := contractObject(components["securitySchemes"], "components.securitySchemes")
	if securitySchemesError != nil {
		return SecurityScheme{}, securitySchemesError
	}
	rawScheme, exists := securitySchemes[name]
	if !exists {
		return SecurityScheme{}, fmt.Errorf("%w: security scheme %s is not declared", errInvalidContract, name)
	}
	scheme, schemeError := contractObject(rawScheme, "components.securitySchemes."+name)
	if schemeError != nil {
		return SecurityScheme{}, schemeError
	}
	schemeType, typeOK := scheme["type"].(string)
	schemeLocation, locationOK := scheme["in"].(string)
	schemeName, nameOK := scheme["name"].(string)
	if !typeOK || !locationOK || !nameOK {
		return SecurityScheme{}, fmt.Errorf("%w: security scheme %s must declare string type, in, and name fields", errInvalidContract, name)
	}
	return SecurityScheme{Type: schemeType, In: schemeLocation, Name: schemeName}, nil
}

func (contract *Contract) SecurityRequirements(path string, method string) ([][]string, error) {
	operation, operationError := contract.operation(path, method)
	if operationError != nil {
		return nil, operationError
	}
	rawRequirements, exists := operation["security"]
	if !exists {
		return nil, fmt.Errorf("%w: %s %s must declare security explicitly", errInvalidContract, strings.ToUpper(method), path)
	}
	requirementValues, ok := rawRequirements.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s %s security must be an array", errInvalidContract, strings.ToUpper(method), path)
	}
	requirements := make([][]string, 0, len(requirementValues))
	for requirementIndex, rawRequirement := range requirementValues {
		requirement, requirementError := contractObject(
			rawRequirement,
			fmt.Sprintf("%s %s security[%d]", strings.ToUpper(method), path, requirementIndex),
		)
		if requirementError != nil {
			return nil, requirementError
		}
		schemeNames := make([]string, 0, len(requirement))
		for schemeName, rawScopes := range requirement {
			if _, schemeError := contract.SecurityScheme(schemeName); schemeError != nil {
				return nil, schemeError
			}
			scopes, scopesOK := rawScopes.([]any)
			if !scopesOK || len(scopes) != 0 {
				return nil, fmt.Errorf(
					"%w: %s %s security scheme %s must use an empty scopes array",
					errInvalidContract,
					strings.ToUpper(method),
					path,
					schemeName,
				)
			}
			schemeNames = append(schemeNames, schemeName)
		}
		sort.Strings(schemeNames)
		requirements = append(requirements, schemeNames)
	}
	return requirements, nil
}

func (contract *Contract) RequestPropertyNames(path string, method string) ([]string, error) {
	operation, operationError := contract.operation(path, method)
	if operationError != nil {
		return nil, operationError
	}
	requestBody, objectError := contractObject(operation["requestBody"], strings.ToUpper(method)+" "+path+" requestBody")
	if objectError != nil {
		return nil, objectError
	}
	if _, hasReference := requestBody["$ref"]; hasReference {
		requestBody, objectError = contract.resolveReference(requestBody)
		if objectError != nil {
			return nil, objectError
		}
	}
	content, contentError := contractObject(requestBody["content"], strings.ToUpper(method)+" "+path+" requestBody.content")
	if contentError != nil {
		return nil, contentError
	}
	jsonMedia, mediaError := contractObject(content["application/json"], strings.ToUpper(method)+" "+path+" application/json")
	if mediaError != nil {
		return nil, mediaError
	}
	schema, schemaError := contractObject(jsonMedia["schema"], strings.ToUpper(method)+" "+path+" application/json.schema")
	if schemaError != nil {
		return nil, schemaError
	}
	resolvedSchema, resolveError := contract.resolveSchema(schema)
	if resolveError != nil {
		return nil, resolveError
	}
	properties, propertiesError := contractObject(resolvedSchema["properties"], strings.ToUpper(method)+" "+path+" request properties")
	if propertiesError != nil {
		return nil, propertiesError
	}
	propertyNames := make([]string, 0, len(properties))
	for propertyName := range properties {
		propertyNames = append(propertyNames, propertyName)
	}
	sort.Strings(propertyNames)
	return propertyNames, nil
}

func (contract *Contract) ValidateRequest(path string, method string, request *http.Request, body []byte) error {
	operation, operationError := contract.operation(path, method)
	if operationError != nil {
		return operationError
	}
	if requiredContentType, hasRequiredContentType := operation["x-llm-proxy-required-request-content-type"].(string); hasRequiredContentType {
		actualContentType, _, parseError := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if parseError != nil || actualContentType != requiredContentType {
			return fmt.Errorf("%w: %s %s requires Content-Type %s", errInvalidContract, strings.ToUpper(method), path, requiredContentType)
		}
	}
	parameters, parameterError := contract.parameters(path, operation)
	if parameterError != nil {
		return parameterError
	}
	declaredQuery := map[string]struct{}{}
	for parameterIndex, parameter := range parameters {
		location, _ := parameter["in"].(string)
		name, _ := parameter["name"].(string)
		required, _ := parameter["required"].(bool)
		switch location {
		case "query":
			declaredQuery[name] = struct{}{}
			values, exists := request.URL.Query()[name]
			if required && (!exists || len(values) != 1 || strings.TrimSpace(values[0]) == "") {
				return fmt.Errorf("%w: %s %s missing required query parameter %s", errInvalidContract, strings.ToUpper(method), path, name)
			}
			for _, value := range values {
				if schema, ok := parameter["schema"].(map[string]any); ok {
					if valueError := contract.validateStringValue(schema, value, fmt.Sprintf("parameters[%d] query %s", parameterIndex, name)); valueError != nil {
						return valueError
					}
				}
			}
		case "header":
			if required && strings.TrimSpace(request.Header.Get(name)) == "" {
				return fmt.Errorf("%w: %s %s missing required header %s", errInvalidContract, strings.ToUpper(method), path, name)
			}
		case "path":
			if !required {
				return fmt.Errorf("%w: %s %s path parameter %s must be required", errInvalidContract, strings.ToUpper(method), path, name)
			}
		default:
			return fmt.Errorf("%w: %s %s parameter %s has unsupported location %q", errInvalidContract, strings.ToUpper(method), path, name, location)
		}
	}
	allowUndeclaredQuery, _ := operation["x-llm-proxy-allow-undeclared-query"].(bool)
	if !allowUndeclaredQuery {
		for queryName := range request.URL.Query() {
			if _, declared := declaredQuery[queryName]; !declared {
				return fmt.Errorf("%w: %s %s has undeclared query parameter %s", errInvalidContract, strings.ToUpper(method), path, queryName)
			}
		}
	}

	rawRequestBody, hasRequestBody := operation["requestBody"]
	if !hasRequestBody {
		if len(body) != 0 {
			return fmt.Errorf("%w: %s %s has undocumented request body", errInvalidContract, strings.ToUpper(method), path)
		}
		return nil
	}
	requestBody, bodyError := contractObject(rawRequestBody, strings.ToUpper(method)+" "+path+" requestBody")
	if bodyError != nil {
		return bodyError
	}
	if _, hasReference := requestBody["$ref"]; hasReference {
		requestBody, bodyError = contract.resolveReference(requestBody)
		if bodyError != nil {
			return bodyError
		}
	}
	bodyRequired, _ := requestBody["required"].(bool)
	if bodyRequired && len(body) == 0 {
		return fmt.Errorf("%w: %s %s requires a request body", errInvalidContract, strings.ToUpper(method), path)
	}
	if len(body) == 0 {
		return nil
	}
	mediaType, _, mediaError := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if mediaError != nil {
		return fmt.Errorf("%w: %s %s content type: %v", errInvalidContract, strings.ToUpper(method), path, mediaError)
	}
	content, contentError := contractObject(requestBody["content"], strings.ToUpper(method)+" "+path+" requestBody.content")
	if contentError != nil {
		return contentError
	}
	mediaContract, ok := content[mediaType]
	if !ok {
		return fmt.Errorf("%w: %s %s content type %s is not declared", errInvalidContract, strings.ToUpper(method), path, mediaType)
	}
	mediaObject, mediaObjectError := contractObject(mediaContract, strings.ToUpper(method)+" "+path+" "+mediaType)
	if mediaObjectError != nil {
		return mediaObjectError
	}
	schema, schemaError := contractObject(mediaObject["schema"], strings.ToUpper(method)+" "+path+" "+mediaType+".schema")
	if schemaError != nil {
		return schemaError
	}
	if mediaType != "application/json" {
		return nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decodeError := decoder.Decode(&value); decodeError != nil {
		return fmt.Errorf("%w: %s %s JSON body: %v", errInvalidContract, strings.ToUpper(method), path, decodeError)
	}
	return contract.validateValue(schema, value, strings.ToUpper(method)+" "+path+" request body")
}

func (contract *Contract) ValidateResponse(path string, method string, statusCode int, headers http.Header, body []byte) error {
	operation, operationError := contract.operation(path, method)
	if operationError != nil {
		return operationError
	}
	responses, responsesError := contractObject(operation["responses"], strings.ToUpper(method)+" "+path+" responses")
	if responsesError != nil {
		return responsesError
	}
	status := strconv.Itoa(statusCode)
	responseValue, declared := responses[status]
	if !declared {
		return fmt.Errorf("%w: %s %s status %s is not declared", errInvalidContract, strings.ToUpper(method), path, status)
	}
	response, responseError := contractObject(responseValue, strings.ToUpper(method)+" "+path+" response "+status)
	if responseError != nil {
		return responseError
	}
	if _, hasReference := response["$ref"]; hasReference {
		response, responseError = contract.resolveReference(response)
		if responseError != nil {
			return responseError
		}
	}
	if rawHeaders, hasHeaders := response["headers"]; hasHeaders {
		responseHeaders, headersError := contractObject(rawHeaders, strings.ToUpper(method)+" "+path+" response "+status+" headers")
		if headersError != nil {
			return headersError
		}
		for headerName, rawHeader := range responseHeaders {
			headerContract, headerError := contractObject(rawHeader, "response header "+headerName)
			if headerError != nil {
				return headerError
			}
			if _, hasReference := headerContract["$ref"]; hasReference {
				headerContract, headerError = contract.resolveReference(headerContract)
				if headerError != nil {
					return headerError
				}
			}
			required, _ := headerContract["x-required"].(bool)
			if required && strings.TrimSpace(headers.Get(headerName)) == "" {
				return fmt.Errorf("%w: %s %s response %s missing required header %s", errInvalidContract, strings.ToUpper(method), path, status, headerName)
			}
		}
	}
	rawContent, hasContent := response["content"]
	if !hasContent {
		if len(body) != 0 {
			return fmt.Errorf("%w: %s %s response %s has undocumented body", errInvalidContract, strings.ToUpper(method), path, status)
		}
		return nil
	}
	content, contentError := contractObject(rawContent, strings.ToUpper(method)+" "+path+" response "+status+" content")
	if contentError != nil {
		return contentError
	}
	mediaType, _, mediaError := mime.ParseMediaType(headers.Get("Content-Type"))
	if mediaError != nil {
		return fmt.Errorf("%w: %s %s response %s content type: %v", errInvalidContract, strings.ToUpper(method), path, status, mediaError)
	}
	rawMediaContract, declaredMedia := content[mediaType]
	if !declaredMedia {
		return fmt.Errorf("%w: %s %s response %s content type %s is not declared", errInvalidContract, strings.ToUpper(method), path, status, mediaType)
	}
	mediaContract, mediaContractError := contractObject(rawMediaContract, strings.ToUpper(method)+" "+path+" response "+status+" "+mediaType)
	if mediaContractError != nil {
		return mediaContractError
	}
	schema, schemaError := contractObject(mediaContract["schema"], strings.ToUpper(method)+" "+path+" response "+status+" "+mediaType+".schema")
	if schemaError != nil {
		return schemaError
	}
	if mediaType != "application/json" {
		return contract.validateValue(schema, string(body), strings.ToUpper(method)+" "+path+" response "+status+" body")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decodeError := decoder.Decode(&value); decodeError != nil {
		return fmt.Errorf("%w: %s %s response %s JSON body: %v", errInvalidContract, strings.ToUpper(method), path, status, decodeError)
	}
	return contract.validateValue(schema, value, strings.ToUpper(method)+" "+path+" response "+status+" body")
}

func (contract *Contract) operation(path string, method string) (map[string]any, error) {
	paths, pathsError := contractObject(contract.document["paths"], "paths")
	if pathsError != nil {
		return nil, pathsError
	}
	pathItemValue, exists := paths[path]
	if !exists {
		return nil, fmt.Errorf("%w: operation path %s is not declared", errInvalidContract, path)
	}
	pathItem, pathItemError := contractObject(pathItemValue, "paths."+path)
	if pathItemError != nil {
		return nil, pathItemError
	}
	normalizedMethod := strings.ToLower(method)
	operationValue, exists := pathItem[normalizedMethod]
	if !exists {
		return nil, fmt.Errorf("%w: operation %s %s is not declared", errInvalidContract, strings.ToUpper(method), path)
	}
	return contractObject(operationValue, "paths."+path+"."+normalizedMethod)
}

func (contract *Contract) parameters(path string, operation map[string]any) ([]map[string]any, error) {
	rawParameters, hasParameters := operation["parameters"]
	if !hasParameters {
		return nil, nil
	}
	parameterValues, ok := rawParameters.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: operation %s parameters must be an array", errInvalidContract, path)
	}
	parameters := make([]map[string]any, 0, len(parameterValues))
	for parameterIndex, rawParameter := range parameterValues {
		parameter, parameterError := contractObject(rawParameter, fmt.Sprintf("operation %s parameters[%d]", path, parameterIndex))
		if parameterError != nil {
			return nil, parameterError
		}
		if _, hasReference := parameter["$ref"]; hasReference {
			resolvedParameter, resolveError := contract.resolveReference(parameter)
			if resolveError != nil {
				return nil, resolveError
			}
			parameter = resolvedParameter
		}
		parameters = append(parameters, parameter)
	}
	return parameters, nil
}

func (contract *Contract) resolveSchema(schema map[string]any) (map[string]any, error) {
	if _, hasReference := schema["$ref"]; !hasReference {
		return schema, nil
	}
	return contract.resolveReference(schema)
}

func (contract *Contract) resolveReference(referenceObject map[string]any) (map[string]any, error) {
	reference, ok := referenceObject["$ref"].(string)
	if !ok || !strings.HasPrefix(reference, "#/") {
		return nil, fmt.Errorf("%w: unsupported reference %v", errInvalidContract, referenceObject["$ref"])
	}
	var current any = contract.document
	for _, escapedSegment := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		segment := strings.ReplaceAll(strings.ReplaceAll(escapedSegment, "~1", "/"), "~0", "~")
		currentObject, objectError := contractObject(current, "reference "+reference)
		if objectError != nil {
			return nil, objectError
		}
		next, exists := currentObject[segment]
		if !exists {
			return nil, fmt.Errorf("%w: unresolved reference %s", errInvalidContract, reference)
		}
		current = next
	}
	return contractObject(current, "reference "+reference)
}

func (contract *Contract) validateValue(schema map[string]any, value any, context string) error {
	resolvedSchema, resolveError := contract.resolveSchema(schema)
	if resolveError != nil {
		return resolveError
	}
	if oneOfValues, hasOneOf := resolvedSchema["oneOf"].([]any); hasOneOf {
		matchCount := 0
		var lastError error
		for alternativeIndex, rawAlternative := range oneOfValues {
			alternative, alternativeError := contractObject(rawAlternative, fmt.Sprintf("%s.oneOf[%d]", context, alternativeIndex))
			if alternativeError != nil {
				return alternativeError
			}
			if validationError := contract.validateValue(alternative, value, context); validationError == nil {
				matchCount++
			} else {
				lastError = validationError
			}
		}
		if matchCount != 1 {
			return fmt.Errorf("%w: %s matched %d oneOf alternatives: %v", errInvalidContract, context, matchCount, lastError)
		}
		return nil
	}
	schemaType, _ := resolvedSchema["type"].(string)
	switch schemaType {
	case "object":
		objectValue, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %s must be an object", errInvalidContract, context)
		}
		properties, propertiesError := contractObject(resolvedSchema["properties"], context+".properties")
		if propertiesError != nil {
			return propertiesError
		}
		if rawRequired, hasRequired := resolvedSchema["required"].([]any); hasRequired {
			for requiredIndex, rawRequiredName := range rawRequired {
				requiredName, ok := rawRequiredName.(string)
				if !ok {
					return fmt.Errorf("%w: %s.required[%d] must be a string", errInvalidContract, context, requiredIndex)
				}
				if _, exists := objectValue[requiredName]; !exists {
					return fmt.Errorf("%w: %s missing required property %s", errInvalidContract, context, requiredName)
				}
			}
		}
		additionalProperties, hasAdditionalProperties := resolvedSchema["additionalProperties"].(bool)
		for propertyName, propertyValue := range objectValue {
			rawPropertySchema, declared := properties[propertyName]
			if !declared {
				if hasAdditionalProperties && !additionalProperties {
					return fmt.Errorf("%w: %s has undeclared property %s", errInvalidContract, context, propertyName)
				}
				continue
			}
			propertySchema, propertySchemaError := contractObject(rawPropertySchema, context+"."+propertyName)
			if propertySchemaError != nil {
				return propertySchemaError
			}
			if propertyError := contract.validateValue(propertySchema, propertyValue, context+"."+propertyName); propertyError != nil {
				return propertyError
			}
		}
		return nil
	case "array":
		arrayValue, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%w: %s must be an array", errInvalidContract, context)
		}
		if rawMinimumItems, hasMinimumItems := resolvedSchema["minItems"].(json.Number); hasMinimumItems {
			minimumItems, conversionError := strconv.Atoi(string(rawMinimumItems))
			if conversionError != nil {
				return fmt.Errorf("%w: %s minItems: %v", errInvalidContract, context, conversionError)
			}
			if len(arrayValue) < minimumItems {
				return fmt.Errorf("%w: %s must contain at least %d items", errInvalidContract, context, minimumItems)
			}
		}
		itemSchema, itemSchemaError := contractObject(resolvedSchema["items"], context+".items")
		if itemSchemaError != nil {
			return itemSchemaError
		}
		for itemIndex, item := range arrayValue {
			if itemError := contract.validateValue(itemSchema, item, fmt.Sprintf("%s[%d]", context, itemIndex)); itemError != nil {
				return itemError
			}
		}
		return nil
	case "string":
		stringValue, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: %s must be a string", errInvalidContract, context)
		}
		return validateStringSchema(resolvedSchema, stringValue, context)
	case "integer":
		numberValue, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("%w: %s must be an integer", errInvalidContract, context)
		}
		integerValue, integerError := numberValue.Int64()
		if integerError != nil {
			return fmt.Errorf("%w: %s must be an integer: %v", errInvalidContract, context, integerError)
		}
		if rawMinimum, hasMinimum := resolvedSchema["minimum"].(json.Number); hasMinimum {
			minimum, minimumError := rawMinimum.Int64()
			if minimumError != nil {
				return fmt.Errorf("%w: %s minimum: %v", errInvalidContract, context, minimumError)
			}
			if integerValue < minimum {
				return fmt.Errorf("%w: %s must be at least %d", errInvalidContract, context, minimum)
			}
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%w: %s must be a boolean", errInvalidContract, context)
		}
		return nil
	default:
		return fmt.Errorf("%w: %s has unsupported schema type %q", errInvalidContract, context, schemaType)
	}
}

func (contract *Contract) validateStringValue(schema map[string]any, value string, context string) error {
	resolvedSchema, resolveError := contract.resolveSchema(schema)
	if resolveError != nil {
		return resolveError
	}
	schemaType, _ := resolvedSchema["type"].(string)
	switch schemaType {
	case "string":
		return validateStringSchema(resolvedSchema, value, context)
	case "integer":
		integerValue, conversionError := strconv.ParseInt(value, 10, 64)
		if conversionError != nil {
			return fmt.Errorf("%w: %s must be an integer", errInvalidContract, context)
		}
		return contract.validateValue(resolvedSchema, json.Number(strconv.FormatInt(integerValue, 10)), context)
	case "boolean":
		if value != "true" && value != "false" {
			return fmt.Errorf("%w: %s must be true or false", errInvalidContract, context)
		}
		return nil
	default:
		return fmt.Errorf("%w: %s has unsupported query schema type %q", errInvalidContract, context, schemaType)
	}
}

func validateStringSchema(schema map[string]any, value string, context string) error {
	if constant, hasConstant := schema["const"].(string); hasConstant && value != constant {
		return fmt.Errorf("%w: %s value=%q want=%q", errInvalidContract, context, value, constant)
	}
	if rawMinimumLength, hasMinimumLength := schema["minLength"].(json.Number); hasMinimumLength {
		minimumLength, conversionError := strconv.Atoi(string(rawMinimumLength))
		if conversionError != nil {
			return fmt.Errorf("%w: %s minLength: %v", errInvalidContract, context, conversionError)
		}
		if len(value) < minimumLength {
			return fmt.Errorf("%w: %s must contain at least %d bytes", errInvalidContract, context, minimumLength)
		}
	}
	if rawEnum, hasEnum := schema["enum"].([]any); hasEnum {
		for _, candidate := range rawEnum {
			if candidate == value {
				return nil
			}
		}
		return fmt.Errorf("%w: %s value %q is outside enum", errInvalidContract, context, value)
	}
	return nil
}

func contractObject(value any, context string) (map[string]any, error) {
	objectValue, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be an object", errInvalidContract, context)
	}
	return objectValue, nil
}

func isHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return true
	default:
		return false
	}
}
