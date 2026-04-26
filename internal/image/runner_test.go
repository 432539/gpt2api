package image

import (
	"reflect"
	"testing"

	"github.com/432539/gpt2api/internal/upstream/chatgpt"
)

func TestFilterOutReferenceFileIDs(t *testing.T) {
	refs := []*chatgpt.UploadedFile{
		{FileID: "file_uploaded_ref"},
		{FileID: " sed:file_uploaded_ref_with_prefix "},
		nil,
	}

	refSet := referenceUploadFileIDSet(refs)
	got := filterOutReferenceFileIDs([]string{
		"file_uploaded_ref",
		"file_generated_result",
		"sed:file_uploaded_ref",
		"file_uploaded_ref_with_prefix",
		"sed:file_generated_sediment",
	}, refSet)

	want := []string{
		"file_generated_result",
		"sed:file_uploaded_ref",
		"sed:file_generated_sediment",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterOutReferenceFileIDs() = %#v, want %#v", got, want)
	}
}
