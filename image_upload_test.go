package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestUploadImageCropsOptimizesAndRenamesImage(t *testing.T) {
	previousWebRoot := webRoot
	webRoot = t.TempDir()
	t.Cleanup(func() {
		webRoot = previousWebRoot
	})

	source := image.NewRGBA(image.Rect(0, 0, 1600, 900))
	random := rand.New(rand.NewSource(42))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.SetRGBA(x, y, color.RGBA{
				R: uint8(random.Intn(256)),
				G: uint8(random.Intn(256)),
				B: uint8(random.Intn(256)),
				A: 255,
			})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, source); err != nil {
		t.Fatal(err)
	}

	response := requestImageUpload(t, "../../same-name.png", input.Bytes())
	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, response.Code, response.Body.String())
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Base(result.URL)
	if matched, err := regexp.MatchString(`^\d{8}T\d{6}\.\d{9}Z(?:-\d+)?\.jpg$`, filename); err != nil || !matched {
		t.Fatalf("expected a timestamp-based JPEG filename, got %q", filename)
	}
	if result.URL == "/upload/same-name.png" {
		t.Fatal("upload retained the client-provided filename")
	}

	optimized, err := os.ReadFile(filepath.Join(webRoot, "upload", filename))
	if err != nil {
		t.Fatal(err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(optimized))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" {
		t.Fatalf("expected optimized JPEG output, got %q", format)
	}
	if config.Width != 1200 || config.Height != 900 {
		t.Fatalf("expected a 1200x900 4:3 image, got %dx%d", config.Width, config.Height)
	}
	if len(optimized) >= input.Len() {
		t.Fatalf("optimized file is not smaller than the original: %d >= %d", len(optimized), input.Len())
	}
}

func TestUploadImageRejectsInvalidImage(t *testing.T) {
	previousWebRoot := webRoot
	webRoot = t.TempDir()
	t.Cleanup(func() {
		webRoot = previousWebRoot
	})

	response := requestImageUpload(t, "not-an-image.txt", []byte("not an image"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
	}
}

func TestSaveRecipeImageDoesNotOverwriteSameTimestamp(t *testing.T) {
	uploadDir := t.TempDir()
	now := time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC)

	first, err := saveRecipeImage(uploadDir, []byte("first"), now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := saveRecipeImage(uploadDir, []byte("second"), now)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("same-timestamp uploads received the same filename %q", first)
	}

	for filename, want := range map[string]string{first: "first", second: "second"} {
		got, err := os.ReadFile(filepath.Join(uploadDir, filename))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("file %q contains %q, want %q", filename, got, want)
		}
	}
}

func requestImageUpload(t *testing.T, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router := chi.NewRouter()
	router.Post("/api/upload", uploadImage)
	router.ServeHTTP(response, request)
	return response
}
