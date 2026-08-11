package messages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type payloadPathPart struct {
	key   string
	index *int
}

type PayloadFilter struct {
	parts    []payloadPathPart
	operator string
	expected any
}

func CompilePayloadFilter(expression string) (PayloadFilter, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return PayloadFilter{}, nil
	}
	expression = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(expression, "jsonpath:"), "jmespath:"))
	path, operator, expectedText := splitPayloadExpression(expression)
	path = strings.TrimSpace(strings.TrimPrefix(path, "$"))
	path = strings.TrimPrefix(path, ".")
	parts, err := parsePayloadPath(path)
	if err != nil {
		return PayloadFilter{}, err
	}
	filter := PayloadFilter{parts: parts, operator: operator}
	if operator != "" {
		if err := json.Unmarshal([]byte(expectedText), &filter.expected); err != nil {
			filter.expected = strings.Trim(strings.TrimSpace(expectedText), "'\"")
		}
	}
	return filter, nil
}

func splitPayloadExpression(expression string) (string, string, string) {
	for _, operator := range []string{"!=", "=="} {
		if before, after, found := strings.Cut(expression, operator); found {
			return before, operator, strings.TrimSpace(after)
		}
	}
	return expression, "", ""
}

func parsePayloadPath(path string) ([]payloadPathPart, error) {
	if path == "" {
		return nil, fmt.Errorf("payload filter path is required")
	}
	normalized := strings.ReplaceAll(path, "['", ".")
	normalized = strings.ReplaceAll(normalized, "']", "")
	segments := strings.Split(normalized, ".")
	parts := make([]payloadPathPart, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			return nil, fmt.Errorf("payload filter path %q is invalid", path)
		}
		for segment != "" {
			open := strings.IndexByte(segment, '[')
			if open < 0 {
				parts = append(parts, payloadPathPart{key: segment})
				break
			}
			if open > 0 {
				parts = append(parts, payloadPathPart{key: segment[:open]})
			}
			close := strings.IndexByte(segment[open:], ']')
			if close < 0 {
				return nil, fmt.Errorf("payload filter path %q has an unclosed index", path)
			}
			close += open
			index, err := strconv.Atoi(segment[open+1 : close])
			if err != nil || index < 0 {
				return nil, fmt.Errorf("payload filter path %q has an invalid index", path)
			}
			parts = append(parts, payloadPathPart{index: &index})
			segment = segment[close+1:]
		}
	}
	return parts, nil
}

func (f PayloadFilter) Empty() bool { return len(f.parts) == 0 }

func (f PayloadFilter) Match(payload []byte) bool {
	if f.Empty() {
		return true
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	for _, part := range f.parts {
		if part.index != nil {
			items, ok := value.([]any)
			if !ok || *part.index >= len(items) {
				return false
			}
			value = items[*part.index]
			continue
		}
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		value, ok = object[part.key]
		if !ok {
			return false
		}
	}
	switch f.operator {
	case "==":
		return equalPayloadValue(value, f.expected)
	case "!=":
		return !equalPayloadValue(value, f.expected)
	default:
		return payloadTruthy(value)
	}
}

func equalPayloadValue(left, right any) bool {
	if leftNumber, leftOK := payloadNumber(left); leftOK {
		if rightNumber, rightOK := payloadNumber(right); rightOK {
			return leftNumber == rightNumber
		}
	}
	return reflect.DeepEqual(left, right)
}

func payloadNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	case float64:
		return value, true
	default:
		return 0, false
	}
}

func payloadTruthy(value any) bool {
	switch value := value.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		return value != ""
	case []any:
		return len(value) > 0
	case map[string]any:
		return len(value) > 0
	default:
		return true
	}
}
