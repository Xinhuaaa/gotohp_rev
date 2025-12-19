package backend

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxDebugDecodeDepth = 30

func appendFieldValue(m map[string]any, fieldNum int, value any) {
	key := fmt.Sprintf("%d", fieldNum)
	if existing, ok := m[key]; ok {
		switch vv := existing.(type) {
		case []any:
			m[key] = append(vv, value)
		default:
			m[key] = []any{vv, value}
		}
		return
	}
	m[key] = value
}

func bufferObject(data []byte) map[string]any {
	parts := make([]string, 0, len(data))
	for _, b := range data {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	out := map[string]any{
		"Type":   fmt.Sprintf("Buffer (%d bytes)", len(data)),
		"As hex": strings.Join(parts, " "),
	}
	// Commonly needed for debugging requests.
	out["As base64"] = base64.StdEncoding.EncodeToString(data)
	if utf8.Valid(data) {
		out["As string"] = string(data)
	}
	return out
}

func decodeProtobufMessage(data []byte, depth int) (map[string]any, bool) {
	if depth > maxDebugDecodeDepth {
		return map[string]any{}, true
	}

	out := map[string]any{}
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 || fieldNum <= 0 {
			return nil, false
		}
		offset = newOffset

		switch wireType {
		case 0: // varint
			v, n := readVarint(data, offset)
			if n < 0 {
				return nil, false
			}
			offset = n
			appendFieldValue(out, fieldNum, int64(v))
		case 1: // 64-bit
			if offset+8 > len(data) {
				return nil, false
			}
			appendFieldValue(out, fieldNum, bufferObject(data[offset:offset+8]))
			offset += 8
		case 2: // length-delimited
			l, n := readVarint(data, offset)
			if n < 0 || n+int(l) > len(data) {
				return nil, false
			}
			fieldData := data[n : n+int(l)]
			offset = n + int(l)

			// Prefer nested-message decoding when it looks plausible.
			if len(fieldData) > 0 {
				if nested, ok := decodeProtobufMessage(fieldData, depth+1); ok && len(nested) > 0 {
					appendFieldValue(out, fieldNum, nested)
					continue
				}
			}

			if isPrintableString(fieldData) {
				appendFieldValue(out, fieldNum, string(fieldData))
				continue
			}
			appendFieldValue(out, fieldNum, bufferObject(fieldData))
		case 3: // start group
			group, n, ok := decodeProtobufGroup(data, offset, depth+1, fieldNum)
			if !ok {
				return nil, false
			}
			offset = n
			appendFieldValue(out, fieldNum, group)
		case 4: // end group (unexpected)
			return out, true
		case 5: // 32-bit
			if offset+4 > len(data) {
				return nil, false
			}
			appendFieldValue(out, fieldNum, bufferObject(data[offset:offset+4]))
			offset += 4
		default:
			return nil, false
		}
	}

	return out, true
}

func decodeProtobufGroup(data []byte, offset int, depth int, groupFieldNum int) (map[string]any, int, bool) {
	if depth > maxDebugDecodeDepth {
		return map[string]any{}, offset, true
	}

	out := map[string]any{}
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 || fieldNum <= 0 {
			return nil, offset, false
		}
		offset = newOffset

		if wireType == 4 && fieldNum == groupFieldNum {
			return out, offset, true
		}

		switch wireType {
		case 0:
			v, n := readVarint(data, offset)
			if n < 0 {
				return nil, offset, false
			}
			offset = n
			appendFieldValue(out, fieldNum, int64(v))
		case 1:
			if offset+8 > len(data) {
				return nil, offset, false
			}
			appendFieldValue(out, fieldNum, bufferObject(data[offset:offset+8]))
			offset += 8
		case 2:
			l, n := readVarint(data, offset)
			if n < 0 || n+int(l) > len(data) {
				return nil, offset, false
			}
			fieldData := data[n : n+int(l)]
			offset = n + int(l)

			if len(fieldData) > 0 {
				if nested, ok := decodeProtobufMessage(fieldData, depth+1); ok && len(nested) > 0 {
					appendFieldValue(out, fieldNum, nested)
					continue
				}
			}
			if isPrintableString(fieldData) {
				appendFieldValue(out, fieldNum, string(fieldData))
				continue
			}
			appendFieldValue(out, fieldNum, bufferObject(fieldData))
		case 3:
			group, n, ok := decodeProtobufGroup(data, offset, depth+1, fieldNum)
			if !ok {
				return nil, offset, false
			}
			offset = n
			appendFieldValue(out, fieldNum, group)
		case 4:
			// end-group for someone else
			return out, offset, true
		case 5:
			if offset+4 > len(data) {
				return nil, offset, false
			}
			appendFieldValue(out, fieldNum, bufferObject(data[offset:offset+4]))
			offset += 4
		default:
			return nil, offset, false
		}
	}

	return nil, offset, false
}
