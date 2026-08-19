package mcpkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// Run invokes the CLI projection. Args begin with the tool name; the product
// owns the parent "run" command and passes its remaining arguments here.
func Run(ctx context.Context, registry *Registry, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("mcp-kit: tool name is required")
	}
	tool, ok := registry.lookup(args[0])
	if !ok {
		return fmt.Errorf("mcp-kit: unknown tool %q", args[0])
	}

	jsonOutput := false
	toolArgs := make([]string, 0, len(args)-1)
	parseGlobalOptions := true
	for _, arg := range args[1:] {
		if parseGlobalOptions && arg == "--" {
			parseGlobalOptions = false
			toolArgs = append(toolArgs, arg)
			continue
		}
		if parseGlobalOptions && arg == "--json" {
			jsonOutput = true
			continue
		}
		toolArgs = append(toolArgs, arg)
	}
	if stderr == nil {
		stderr = io.Discard
	}
	ctx = withProgressReporter(ctx, func(_ context.Context, progress Progress) error {
		label := progress.Message
		if label == "" {
			label = "progress"
		}
		if progress.Total > 0 {
			_, err := fmt.Fprintf(stderr, "%s %g/%g\n", label, progress.Current, progress.Total)
			return err
		}
		_, err := fmt.Fprintf(stderr, "%s %g\n", label, progress.Current)
		return err
	})
	output, err := tool.invoke(ctx, toolArgs)
	if err != nil {
		return err
	}
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(output)
	}
	text, err := tool.renderText(output)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, text)
	return err
}

type cliField struct {
	path      []int
	name      string
	fieldType reflect.Type
	required  bool
}

type cliParser[In any] struct {
	positionals []cliField
	flags       map[string]cliField
	flagOrder   []string
}

func buildCLIParser[In any]() (*cliParser[In], error) {
	typeOfInput := reflect.TypeOf((*In)(nil)).Elem()
	if typeOfInput.Kind() != reflect.Struct {
		return nil, fmt.Errorf("CLI input must be a struct, got %s", typeOfInput.Kind())
	}
	parser := &cliParser[In]{flags: make(map[string]cliField)}
	if err := parser.addFields(typeOfInput, nil); err != nil {
		return nil, err
	}
	return parser, nil
}

func (parser *cliParser[In]) addFields(inputType reflect.Type, parentPath []int) error {
	for i := 0; i < inputType.NumField(); i++ {
		field := inputType.Field(i)
		if !field.IsExported() {
			continue
		}
		path := append(append([]int(nil), parentPath...), i)
		jsonParts := strings.Split(field.Tag.Get("json"), ",")
		jsonName := jsonParts[0]
		if jsonName == "-" || field.Tag.Get("cli") == "-" {
			continue
		}
		embeddedType := field.Type
		if embeddedType.Kind() == reflect.Pointer {
			embeddedType = embeddedType.Elem()
		}
		if field.Anonymous && jsonName == "" && embeddedType.Kind() == reflect.Struct {
			if err := parser.addFields(embeddedType, path); err != nil {
				return err
			}
			continue
		}
		if jsonName == "" {
			jsonName = strings.ToLower(field.Name)
		}
		if !supportedCLIType(field.Type) {
			return fmt.Errorf("unsupported CLI field %s of type %s", jsonName, field.Type)
		}
		required := true
		for _, option := range jsonParts[1:] {
			if option == "omitempty" || option == "omitzero" {
				required = false
			}
		}
		entry := cliField{path: path, name: jsonName, fieldType: field.Type, required: required}
		if field.Tag.Get("cli") == "positional" {
			if len(parser.positionals) > 0 && isCLISlice(parser.positionals[len(parser.positionals)-1].fieldType) {
				return fmt.Errorf("positional slice %s must be last", parser.positionals[len(parser.positionals)-1].name)
			}
			parser.positionals = append(parser.positionals, entry)
			continue
		}
		name := field.Tag.Get("cli")
		if name == "" {
			name = strings.ReplaceAll(jsonName, "_", "-")
		}
		if name == "json" {
			return fmt.Errorf("CLI flag --json is reserved for output selection")
		}
		if _, exists := parser.flags[name]; exists {
			return fmt.Errorf("duplicate CLI flag --%s", name)
		}
		entry.name = name
		parser.flags[name] = entry
		parser.flagOrder = append(parser.flagOrder, name)
	}
	return nil
}

func supportedCLIType(fieldType reflect.Type) bool {
	_, _, ok := cliFieldShape(fieldType)
	return ok
}

func cliFieldShape(fieldType reflect.Type) (kind reflect.Kind, slice, ok bool) {
	switch fieldType.Kind() {
	case reflect.Pointer:
		kind = fieldType.Elem().Kind()
		return kind, false, supportedCLIScalar(kind)
	case reflect.Slice:
		kind = fieldType.Elem().Kind()
		if kind == reflect.Uint8 {
			return reflect.Invalid, false, false
		}
		return kind, true, supportedCLIScalar(kind)
	default:
		kind = fieldType.Kind()
		return kind, false, supportedCLIScalar(kind)
	}
}

func supportedCLIScalar(kind reflect.Kind) bool {
	switch kind {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func isCLISlice(fieldType reflect.Type) bool {
	_, slice, ok := cliFieldShape(fieldType)
	return ok && slice
}

func (parser *cliParser[In]) parse(args []string) (In, error) {
	var input In
	value := reflect.ValueOf(&input).Elem()

	seenPositionals := 0
	seenFlags := make(map[string]bool)
	parseOptions := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if parseOptions && arg == "--" {
			parseOptions = false
			continue
		}
		if parseOptions && strings.HasPrefix(arg, "--") {
			name, raw, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			entry, exists := parser.flags[name]
			if !exists {
				return input, fmt.Errorf("unknown flag --%s", name)
			}
			field := fieldByPath(value, entry.path)
			if cliScalarKind(field.Type()) == reflect.Bool && !hasValue {
				raw = "true"
				hasValue = true
			}
			if !hasValue {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
					return input, fmt.Errorf("flag --%s requires a value", name)
				}
				i++
				raw = args[i]
			}
			if err := setField(field, raw); err != nil {
				return input, fmt.Errorf("flag --%s: %w", name, err)
			}
			seenFlags[name] = true
			continue
		}

		var entry cliField
		if seenPositionals >= len(parser.positionals) {
			if len(parser.positionals) == 0 || !isCLISlice(parser.positionals[len(parser.positionals)-1].fieldType) {
				return input, fmt.Errorf("unexpected positional argument %q", arg)
			}
			entry = parser.positionals[len(parser.positionals)-1]
		} else {
			entry = parser.positionals[seenPositionals]
		}
		if err := setField(fieldByPath(value, entry.path), arg); err != nil {
			return input, fmt.Errorf("%s: %w", entry.name, err)
		}
		if seenPositionals < len(parser.positionals) {
			seenPositionals++
		}
	}
	if seenPositionals < len(parser.positionals) {
		entry := parser.positionals[seenPositionals]
		if seenPositionals != len(parser.positionals)-1 || !isCLISlice(entry.fieldType) {
			return input, fmt.Errorf("missing positional %s", entry.name)
		}
		field := fieldByPath(value, entry.path)
		if field.IsNil() {
			field.Set(reflect.MakeSlice(field.Type(), 0, 0))
		}
	}
	for _, name := range parser.flagOrder {
		if parser.flags[name].required && !seenFlags[name] {
			return input, fmt.Errorf("missing required flag --%s", name)
		}
	}
	return input, nil
}

func fieldByPath(value reflect.Value, path []int) reflect.Value {
	for index, fieldIndex := range path {
		value = value.Field(fieldIndex)
		if index < len(path)-1 && value.Kind() == reflect.Pointer {
			if value.IsNil() {
				value.Set(reflect.New(value.Type().Elem()))
			}
			value = value.Elem()
		}
	}
	return value
}

func setField(field reflect.Value, raw string) error {
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setField(field.Elem(), raw)
	}
	if field.Kind() == reflect.Slice {
		element := reflect.New(field.Type().Elem()).Elem()
		if err := setField(element, raw); err != nil {
			return err
		}
		field.Set(reflect.Append(field, element))
		return nil
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(raw, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(value)
	default:
		return fmt.Errorf("unsupported CLI field type %s", field.Type())
	}
	return nil
}

func cliScalarKind(fieldType reflect.Type) reflect.Kind {
	kind, _, ok := cliFieldShape(fieldType)
	if !ok {
		return reflect.Invalid
	}
	return kind
}
