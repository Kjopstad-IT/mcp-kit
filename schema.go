package mcpkit

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/google/jsonschema-go/jsonschema"
)

func inputSchemaFor[In any]() (*jsonschema.Schema, error) {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		return nil, fmt.Errorf("infer input schema: %w", err)
	}
	if err := applyMCPHeaderTags(reflect.TypeOf((*In)(nil)).Elem(), schema); err != nil {
		return nil, err
	}
	return schema, nil
}

type schemaField struct {
	field  reflect.StructField
	name   string
	tagged bool
	depth  int
	path   string
}

func applyMCPHeaderTags(inputType reflect.Type, schema *jsonschema.Schema) error {
	if inputType.Kind() == reflect.Pointer {
		inputType = inputType.Elem()
	}
	if inputType.Kind() != reflect.Struct {
		return nil
	}
	fields := collectSchemaFields(inputType, 0, "")
	dominant := dominantSchemaFields(fields)
	seenHeaders := make(map[string]string)
	for _, candidate := range fields {
		field := candidate.field
		header := field.Tag.Get("mcpheader")
		if header == "" {
			continue
		}
		selected := dominant[candidate.name]
		if selected == nil || selected.path != candidate.path {
			return fmt.Errorf("mcpheader field %s is shadowed or ambiguous for JSON name %q", candidate.path, candidate.name)
		}
		if !validHeaderFieldType(field.Type) {
			return fmt.Errorf("mcpheader %q on field %s requires a string, integer, or boolean", header, candidate.path)
		}
		if !validHeaderName(header) {
			return fmt.Errorf("invalid mcpheader %q on field %s", header, candidate.path)
		}
		folded := strings.ToLower(header)
		if prior, exists := seenHeaders[folded]; exists {
			return fmt.Errorf("duplicate mcpheader %q on fields %s and %s", header, prior, candidate.path)
		}
		seenHeaders[folded] = candidate.path
		property := schema.Properties[candidate.name]
		if property == nil {
			return fmt.Errorf("mcpheader field %s is absent from inferred input schema", candidate.path)
		}
		if property.Extra == nil {
			property.Extra = make(map[string]any)
		}
		property.Extra["x-mcp-header"] = header
	}
	return nil
}

func collectSchemaFields(inputType reflect.Type, depth int, parent string) []schemaField {
	var fields []schemaField
	for i := 0; i < inputType.NumField(); i++ {
		field := inputType.Field(i)
		if !field.IsExported() {
			continue
		}
		jsonParts := strings.Split(field.Tag.Get("json"), ",")
		jsonName := jsonParts[0]
		if jsonName == "-" {
			continue
		}
		embeddedType := field.Type
		if embeddedType.Kind() == reflect.Pointer {
			embeddedType = embeddedType.Elem()
		}
		path := field.Name
		if parent != "" {
			path = parent + "." + field.Name
		}
		if field.Anonymous && jsonName == "" && embeddedType.Kind() == reflect.Struct {
			fields = append(fields, collectSchemaFields(embeddedType, depth+1, path)...)
			continue
		}
		tagged := jsonName != ""
		if jsonName == "" {
			jsonName = field.Name
		}
		fields = append(fields, schemaField{field: field, name: jsonName, tagged: tagged, depth: depth, path: path})
	}
	return fields
}

func dominantSchemaFields(fields []schemaField) map[string]*schemaField {
	grouped := make(map[string][]schemaField)
	for _, field := range fields {
		grouped[field.name] = append(grouped[field.name], field)
	}
	dominant := make(map[string]*schemaField, len(grouped))
	for name, candidates := range grouped {
		minDepth := candidates[0].depth
		for _, candidate := range candidates[1:] {
			if candidate.depth < minDepth {
				minDepth = candidate.depth
			}
		}
		var shallowest []schemaField
		for _, candidate := range candidates {
			if candidate.depth == minDepth {
				shallowest = append(shallowest, candidate)
			}
		}
		if len(shallowest) == 1 {
			selected := shallowest[0]
			dominant[name] = &selected
			continue
		}
		var tagged []schemaField
		for _, candidate := range shallowest {
			if candidate.tagged {
				tagged = append(tagged, candidate)
			}
		}
		if len(tagged) == 1 {
			selected := tagged[0]
			dominant[name] = &selected
		}
	}
	return dominant
}

func validHeaderFieldType(fieldType reflect.Type) bool {
	if fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	switch fieldType.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if r > unicode.MaxASCII {
				return false
			}
			continue
		}
		if !strings.ContainsRune("!#$%&'*+-.^_`|~", r) {
			return false
		}
	}
	return true
}
