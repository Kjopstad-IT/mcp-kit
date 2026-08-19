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
func Run(ctx context.Context, registry *Registry, args []string, stdout, _ io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("mcp-kit: tool name is required")
	}
	tool, ok := registry.lookup(args[0])
	if !ok {
		return fmt.Errorf("mcp-kit: unknown tool %q", args[0])
	}

	jsonOutput := false
	toolArgs := make([]string, 0, len(args)-1)
	for _, arg := range args[1:] {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		toolArgs = append(toolArgs, arg)
	}
	output, err := tool.invoke(ctx, toolArgs)
	if err != nil {
		return err
	}
	if jsonOutput {
		return json.NewEncoder(stdout).Encode(output)
	}
	text, err := tool.renderText(output)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, text)
	return err
}

func parseInput[In any](args []string) (In, error) {
	var input In
	value := reflect.ValueOf(&input).Elem()
	if value.Kind() != reflect.Struct {
		return input, fmt.Errorf("CLI input must be a struct, got %s", value.Kind())
	}
	typeOfInput := value.Type()

	var positionals []int
	flags := make(map[string]int)
	fieldNames := make(map[int]string)
	for i := 0; i < value.NumField(); i++ {
		field := typeOfInput.Field(i)
		if !field.IsExported() {
			continue
		}
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" {
			jsonName = strings.ToLower(field.Name)
		}
		if jsonName == "-" {
			continue
		}
		fieldNames[i] = jsonName
		cliTag := field.Tag.Get("cli")
		switch cliTag {
		case "-":
			continue
		case "positional":
			positionals = append(positionals, i)
		default:
			name := cliTag
			if name == "" {
				name = strings.ReplaceAll(jsonName, "_", "-")
			}
			flags[name] = i
		}
	}

	seenPositionals := 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			name, raw, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			fieldIndex, exists := flags[name]
			if !exists {
				return input, fmt.Errorf("unknown flag --%s", name)
			}
			field := value.Field(fieldIndex)
			if field.Kind() == reflect.Bool && !hasValue {
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
			continue
		}

		if seenPositionals >= len(positionals) {
			return input, fmt.Errorf("unexpected positional argument %q", arg)
		}
		fieldIndex := positionals[seenPositionals]
		if err := setField(value.Field(fieldIndex), arg); err != nil {
			return input, fmt.Errorf("%s: %w", fieldNames[fieldIndex], err)
		}
		seenPositionals++
	}
	if seenPositionals < len(positionals) {
		return input, fmt.Errorf("missing positional %s", fieldNames[positionals[seenPositionals]])
	}
	return input, nil
}

func setField(field reflect.Value, raw string) error {
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
