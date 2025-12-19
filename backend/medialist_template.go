package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
)

// Note: This template mirrors the captured request shape.
// Dynamic fields are applied in buildMediaListRequestFromTemplate():
// - 1.2 (limit)
// - 1.4 (page token, when present)
// - 1.6 (sync token)
// - 1.22.1 (trigger mode)
const mediaListRequestTemplateJSON = `{
  "1": {
    "1": {
      "1": {
        "19": "",
        "20": "",
        "25": "",
        "30": {
          "2": ""
        }
      },
      "3": "",
      "4": "",
      "5": "",
      "6": "",
      "7": "",
      "15": "",
      "16": "",
      "17": "",
      "19": "",
      "20": "",
      "21": {
        "1": ""
      },
      "22": "",
      "25": "",
      "30": "",
      "31": "",
      "32": "",
      "33": "",
      "34": "",
      "36": "",
      "37": "",
      "38": "",
      "39": "",
      "40": "",
      "41": ""
    },
    "2": 50,
    "3": {
      "2": "",
      "3": "",
      "7": "",
      "8": "",
      "14": "",
      "16": "",
      "17": "",
      "18": "",
      "19": "",
      "20": "",
      "21": "",
      "22": "",
      "23": "",
      "27": "",
      "29": "",
      "30": "",
      "31": "",
      "32": "",
      "34": "",
      "37": "",
      "38": "",
      "39": "",
      "41": ""
    },
    "7": 2,
    "11": [
      1,
      2
    ],
    "22": {
      "1": 2
    }
  },
  "2": {
    "1": {
      "1": {
        "1": {
          "1": ""
        },
        "2": ""
      }
    },
    "2": ""
  }
}`

var (
	mediaListTemplateOnce sync.Once
	mediaListTemplateRoot map[string]any
	mediaListTemplateErr  error
)

func getMediaListTemplate() (map[string]any, error) {
	mediaListTemplateOnce.Do(func() {
		mediaListTemplateRoot, mediaListTemplateErr = parseMediaListTemplate()
	})
	if mediaListTemplateErr != nil {
		return nil, mediaListTemplateErr
	}
	return mediaListTemplateRoot, nil
}

func parseMediaListTemplate() (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(mediaListRequestTemplateJSON)))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("failed to parse media list template json: %w", err)
	}
	root, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("media list template root is not an object")
	}
	return root, nil
}