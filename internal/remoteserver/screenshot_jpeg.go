package remoteserver

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
)

// goJPEGCompress decodes a PNG and re-encodes as JPEG at the given quality.
func goJPEGCompress(pngData []byte, outputPath string, quality int) error {
	src, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return fmt.Errorf("png decode: %w", err)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	opts := &jpeg.Options{Quality: quality}
	if err := jpeg.Encode(f, src, opts); err != nil {
		return fmt.Errorf("jpeg encode: %w", err)
	}
	return nil
}
