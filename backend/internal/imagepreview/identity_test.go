package imagepreview

import (
	"testing"
)

func TestPreviewETagIncludesEverySourceIdentityField(t *testing.T) {
	data := pngFixture(t)
	base := previewRequest(data)
	service := &Service{}
	original, err := service.ETag(base)
	if err != nil {
		t.Fatal(err)
	}
	variants := map[string]func(*Request){
		"connection":   func(value *Request) { value.ConnectionInstanceID = "other-instance" },
		"root path":    func(value *Request) { value.RootAbsolutePath = "/other-root" },
		"revision":     func(value *Request) { value.RootRevision = "root-2" },
		"relative":     func(value *Request) { value.RelativePath = "nested/image.png" },
		"source token": func(value *Request) { value.SourceToken = "token-2" },
		"source size":  func(value *Request) { value.SourceSize++ },
	}
	for name, mutate := range variants {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			etag, err := service.ETag(value)
			if err != nil {
				t.Fatal(err)
			}
			if etag == original {
				t.Fatalf("identity mutation did not change ETag %q", etag)
			}
		})
	}
	if original == `"token-1"` {
		t.Fatal("preview ETag must not reuse the source ETag")
	}
}
