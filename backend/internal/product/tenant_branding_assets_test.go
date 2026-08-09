package product

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func pngLogo(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestValidateTenantLogoUsesSignatureAndDimensions(t *testing.T) {
	data := pngLogo(t, 128, 64)
	contentType, width, height, err := validateTenantLogo(data, "image/png")
	if err != nil || contentType != "image/png" || width != 128 || height != 64 {
		t.Fatalf("unexpected valid logo result: type=%q width=%d height=%d err=%v", contentType, width, height, err)
	}
	for name, test := range map[string]struct {
		data     []byte
		declared string
	}{
		"forged mime":      {[]byte("not an image"), "image/png"},
		"mismatched mime":  {data, "image/jpeg"},
		"undersized image": {pngLogo(t, 15, 64), "image/png"},
		"oversized image":  {pngLogo(t, 2049, 16), "image/png"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := validateTenantLogo(test.data, test.declared); err == nil {
				t.Fatal("expected logo validation error")
			}
		})
	}
}
