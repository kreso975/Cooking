package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"time"

	xdraw "golang.org/x/image/draw"
)

const (
	maxRecipeImageUploadBytes = 10 << 20
	maxRecipeImagePixels      = 20_000_000
	maxRecipeImageWidth       = 1200
	maxRecipeImageHeight      = 900
)

func optimizeRecipeImage(data []byte) ([]byte, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image configuration: %w", err)
	}
	if format != "jpeg" && format != "png" && format != "gif" {
		return nil, fmt.Errorf("unsupported image format %q", format)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxRecipeImagePixels {
		return nil, fmt.Errorf("image dimensions exceed the supported limit")
	}

	if format == "jpeg" {
		data, err = normalizeJPEGOrientation(data)
		if err != nil {
			return nil, fmt.Errorf("normalize JPEG orientation: %w", err)
		}
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := source.Bounds()
	crop := bounds
	if bounds.Dx()*3 > bounds.Dy()*4 {
		cropWidth := bounds.Dy() * 4 / 3
		crop.Min.X += (bounds.Dx() - cropWidth) / 2
		crop.Max.X = crop.Min.X + cropWidth
	} else {
		cropHeight := bounds.Dx() * 3 / 4
		crop.Min.Y += (bounds.Dy() - cropHeight) / 2
		crop.Max.Y = crop.Min.Y + cropHeight
	}
	if crop.Empty() {
		return nil, fmt.Errorf("image is too small to crop")
	}

	scale := math.Min(1, math.Min(
		float64(maxRecipeImageWidth)/float64(crop.Dx()),
		float64(maxRecipeImageHeight)/float64(crop.Dy()),
	))
	outputWidth := max(1, int(math.Round(float64(crop.Dx())*scale)))
	outputHeight := max(1, int(math.Round(float64(crop.Dy())*scale)))
	cropped := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(cropped, cropped.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(cropped, cropped.Bounds(), source, crop.Min, draw.Over)

	output := cropped
	if outputWidth != crop.Dx() || outputHeight != crop.Dy() {
		output = image.NewRGBA(image.Rect(0, 0, outputWidth, outputHeight))
		xdraw.CatmullRom.Scale(output, output.Bounds(), cropped, cropped.Bounds(), xdraw.Over, nil)
	}

	var optimized bytes.Buffer
	if err := jpeg.Encode(&optimized, output, &jpeg.Options{Quality: 82}); err != nil {
		return nil, fmt.Errorf("encode optimized JPEG: %w", err)
	}
	return optimized.Bytes(), nil
}

func saveRecipeImage(uploadDir string, data []byte, now time.Time) (string, error) {
	timestamp := now.UTC().Format("20060102T150405.000000000Z")
	for suffix := 0; ; suffix++ {
		filename := fmt.Sprintf("%s.jpg", timestamp)
		if suffix > 0 {
			filename = fmt.Sprintf("%s-%d.jpg", timestamp, suffix)
		}

		file, err := os.OpenFile(filepath.Join(uploadDir, filename), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create image file: %w", err)
		}

		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(filepath.Join(uploadDir, filename))
			return "", errors.Join(writeErr, closeErr)
		}
		return filename, nil
	}
}
