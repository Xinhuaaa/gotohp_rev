package backend

import (
	"bytes"
	"testing"
)

func TestParseMediaListResponse_SkipsGroups(t *testing.T) {
	// Build a minimal protobuf response:
	// top-level:
	//   - group field 55 (unknown)
	//   - field 1 (message):
	//       - field 2 (item1)
	//       - group field 99 (unknown)
	//       - field 2 (item2)
	//       - field 2 (item3)

	buildItem := func(mediaKey string) []byte {
		var item bytes.Buffer
		writeProtobufString(&item, 1, mediaKey)
		return item.Bytes()
	}

	buildGroup := func(fieldNum int) []byte {
		var g bytes.Buffer
		startTag := uint64((fieldNum << 3) | 3)
		endTag := uint64((fieldNum << 3) | 4)
		writeVarint(&g, startTag)
		writeProtobufVarint(&g, 1, 1)
		writeVarint(&g, endTag)
		return g.Bytes()
	}

	var field1 bytes.Buffer
	writeProtobufField(&field1, 2, buildItem("AF1Qip_TEST_KEY_1"))
	field1.Write(buildGroup(99))
	writeProtobufField(&field1, 2, buildItem("AF1Qip_TEST_KEY_2"))
	writeProtobufField(&field1, 2, buildItem("AF1Qip_TEST_KEY_3"))

	var top bytes.Buffer
	top.Write(buildGroup(55))
	writeProtobufField(&top, 1, field1.Bytes())

	res, err := parseMediaListResponse(top.Bytes())
	if err != nil {
		t.Fatalf("parseMediaListResponse returned error: %v", err)
	}

	if len(res.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(res.Items))
	}

	if res.Items[0].MediaKey != "AF1Qip_TEST_KEY_1" {
		t.Fatalf("unexpected item[0] media key: %q", res.Items[0].MediaKey)
	}
	if res.Items[1].MediaKey != "AF1Qip_TEST_KEY_2" {
		t.Fatalf("unexpected item[1] media key: %q", res.Items[1].MediaKey)
	}
	if res.Items[2].MediaKey != "AF1Qip_TEST_KEY_3" {
		t.Fatalf("unexpected item[2] media key: %q", res.Items[2].MediaKey)
	}
}
