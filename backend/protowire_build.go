package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

func buildProtobufFromJSONText(jsonText string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(jsonText)))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	root, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected json object at root")
	}
	return buildProtobufFromMap(root)
}

func buildProtobufFromMap(m map[string]any) ([]byte, error) {
	var buf bytes.Buffer

	fieldNums := make([]int, 0, len(m))
	for k := range m {
		n, err := strconv.Atoi(k)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid field number key: %q", k)
		}
		fieldNums = append(fieldNums, n)
	}
	sort.Ints(fieldNums)

	for _, fieldNum := range fieldNums {
		value := m[strconv.Itoa(fieldNum)]
		if value == nil {
			continue
		}
		if err := writeAnyField(&buf, fieldNum, value); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func writeAnyField(buf *bytes.Buffer, fieldNum int, v any) error {
	switch vv := v.(type) {
	case string:
		writeProtobufString(buf, fieldNum, vv)
		return nil
	case int:
		writeProtobufVarint(buf, fieldNum, int64(vv))
		return nil
	case int64:
		writeProtobufVarint(buf, fieldNum, vv)
		return nil
	case int32:
		writeProtobufVarint(buf, fieldNum, int64(vv))
		return nil
	case uint64:
		writeProtobufVarint(buf, fieldNum, int64(vv))
		return nil
	case uint32:
		writeProtobufVarint(buf, fieldNum, int64(vv))
		return nil
	case json.Number:
		i, err := vv.Int64()
		if err != nil {
			return fmt.Errorf("field %d: invalid number %q: %w", fieldNum, vv.String(), err)
		}
		writeProtobufVarint(buf, fieldNum, i)
		return nil
	case float64:
		// Should not happen with UseNumber, but keep it safe.
		writeProtobufVarint(buf, fieldNum, int64(vv))
		return nil
	case map[string]any:
		nested, err := buildProtobufFromMap(vv)
		if err != nil {
			return err
		}
		writeProtobufField(buf, fieldNum, nested)
		return nil
	case []any:
		for _, elem := range vv {
			if err := writeAnyField(buf, fieldNum, elem); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("field %d: unsupported json type %T", fieldNum, v)
	}
}

func deepCopyJSON(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(vv))
		for k, val := range vv {
			m[k] = deepCopyJSON(val)
		}
		return m
	case []any:
		out := make([]any, len(vv))
		for i, val := range vv {
			out[i] = deepCopyJSON(val)
		}
		return out
	default:
		return vv
	}
}

func ensureMapPath(root map[string]any, keys ...string) (map[string]any, error) {
	cur := root
	for _, k := range keys {
		next, ok := cur[k]
		if !ok || next == nil {
			created := map[string]any{}
			cur[k] = created
			cur = created
			continue
		}
		asMap, ok := next.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q is not an object", k)
		}
		cur = asMap
	}
	return cur, nil
}
