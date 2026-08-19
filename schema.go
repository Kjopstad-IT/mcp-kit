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
	seenHeaders := make(map[string]string)
	if err := applyMCPHeaderTags(reflect.TypeOf((*In)(nil)).Elem(), schema, seenHeaders); err != nil {
		return nil, err
	}
	return schema, nil
}

func applyMCPHeaderTags(inputType reflect.Type, schema *jsonschema.Schema, seen map[string]string) error {
	if inputType.Kind() == reflect.Pointer {
		inputType = inputType.Elem()
	}
	if inputType.Kind() != reflect.Struct {
		return nil
	}
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
		if field.Anonymous && jsonName == "" && embeddedType.Kind() == reflect.Struct {
			if err := applyMCPHeaderTags(embeddedType, schema, seen); err != nil {
				return err
			}
			continue
		}
		header := field.Tag.Get("mcpheader")
		if header == "" {
			continue
		}
		if !validHeaderFieldType(field.Type) {
			return fmt.Errorf("mcpheader %q on field %s requires a string, integer, or boolean", header, field.Name)
		}
		if !validHeaderName(header) {
			return fmt.Errorf("invalid mcpheader %q on field %s", header, field.Name)
		}
		folded := strings.ToLower(header)
		if prior, exists := seen[folded]; exists {
			return fmt.Errorf("duplicate mcpheader %q on fields %s and %s", header, prior, field.Name)
		}
		seen[folded] = field.Name
		if jsonName == "" {
			jsonName = field.Name
		}
		property := schema.Properties[jsonName]
		if property == nil {
			return fmt.Errorf("mcpheader field %s is absent from inferred input schema", field.Name)
		}
		if property.Extra == nil {
			property.Extra = make(map[string]any)
		}
		property.Extra["x-mcp-header"] = header
	}
	return nil
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
