package main

import (
	"bytes"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupRecipePDFTest(t *testing.T, recipe Recipe) http.Handler {
	t.Helper()

	previousDB, previousWebRoot := db, webRoot
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	if _, err := testDB.Exec(`CREATE TABLE recipes (
		id TEXT PRIMARY KEY,
		title TEXT,
		slug TEXT,
		category TEXT,
		prep_time TEXT,
		cook_time TEXT,
		servings INTEGER,
		ingredients TEXT,
		steps TEXT,
		image_url TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	_, err = testDB.Exec(
		"INSERT INTO recipes VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		recipe.ID,
		recipe.Title,
		recipe.Slug,
		recipe.Category,
		recipe.PrepTime,
		recipe.CookTime,
		recipe.Servings,
		recipe.Ingredients,
		recipe.Steps,
		recipe.ImageURL,
	)
	if err != nil {
		t.Fatal(err)
	}

	db = testDB
	webRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(webRoot, "upload"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db = previousDB
		webRoot = previousWebRoot
		_ = testDB.Close()
	})

	router := chi.NewRouter()
	router.Get("/api/recipes/{id}/pdf", downloadRecipePDF)
	return router
}

func samplePDFRecipe() Recipe {
	return Recipe{
		ID:          "recipe-1",
		Title:       "Čokoladni kolač",
		Slug:        "cokoladni-kolac",
		Category:    "Dessert",
		PrepTime:    "15 minutes",
		CookTime:    "30 minutes",
		Servings:    6,
		Ingredients: "2 eggs\n100 g sugar",
		Steps:       "Mix the ingredients\nBake until ready",
	}
}

func requestRecipePDF(router http.Handler, id string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/recipes/"+id+"/pdf", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestDownloadRecipePDF(t *testing.T) {
	router := setupRecipePDFTest(t, samplePDFRecipe())
	response := requestRecipePDF(router, "recipe-1")

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("expected application/pdf, got %q", got)
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="cokoladni-kolac.pdf"` {
		t.Fatalf("unexpected Content-Disposition: %q", got)
	}
	if !bytes.HasPrefix(response.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("response does not contain a PDF")
	}
	if !bytes.HasSuffix(bytes.TrimSpace(response.Body.Bytes()), []byte("%%EOF")) {
		t.Fatal("PDF does not have an EOF marker")
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("/XYZ null null 1")) {
		t.Fatal("PDF does not request actual-size initial zoom")
	}
}

func TestDownloadRecipePDFIncludesImage(t *testing.T) {
	recipe := samplePDFRecipe()
	recipe.ImageURL = "/www/upload/recipe.png"
	router := setupRecipePDFTest(t, recipe)

	imageFile, err := os.Create(filepath.Join(webRoot, "upload", "recipe.png"))
	if err != nil {
		t.Fatal(err)
	}
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			imageData.SetRGBA(x, y, color.RGBA{R: 220, G: 120, B: 40, A: 255})
		}
	}
	if err := png.Encode(imageFile, imageData); err != nil {
		_ = imageFile.Close()
		t.Fatal(err)
	}
	if err := imageFile.Close(); err != nil {
		t.Fatal(err)
	}

	response := requestRecipePDF(router, "recipe-1")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("/Subtype /Image")) {
		t.Fatal("generated PDF does not contain the recipe image")
	}
}

func TestDownloadRecipePDFMissingImageFileReturnsError(t *testing.T) {
	recipe := samplePDFRecipe()
	recipe.ImageURL = "/upload/missing.png"
	router := setupRecipePDFTest(t, recipe)

	response := requestRecipePDF(router, "recipe-1")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, response.Code)
	}
	if !strings.Contains(response.Body.String(), "Unable to generate recipe PDF") {
		t.Fatalf("expected explicit PDF error message, got %q", response.Body.String())
	}
}

func TestDownloadRecipePDFUnknownRecipe(t *testing.T) {
	router := setupRecipePDFTest(t, samplePDFRecipe())
	response := requestRecipePDF(router, "missing")

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}

func TestGenerateRecipePDFCreatesMultiplePages(t *testing.T) {
	recipe := samplePDFRecipe()
	recipe.Steps = strings.Repeat("Stir the mixture until it is smooth and evenly combined.\n", 120)

	pdfBytes, err := generateRecipePDF(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if pages := bytes.Count(pdfBytes, []byte("/Type /Page")); pages < 3 {
		t.Fatalf("expected multiple pages for long instructions, found %d page objects", pages)
	}
}

func TestFitPDFImageKeepsEvenBorderAndAspectRatio(t *testing.T) {
	frameWidth, frameHeight, imageWidth, imageHeight := fitPDFImage(1600, 900, 104, 64, 1)

	if frameWidth > 104 || frameHeight > 64 {
		t.Fatalf("image frame exceeds its maximum size: %.2f x %.2f", frameWidth, frameHeight)
	}
	if got, want := frameWidth-imageWidth, 2.0; got != want {
		t.Errorf("horizontal border is %.2f mm, want %.2f mm", got, want)
	}
	if got, want := frameHeight-imageHeight, 2.0; got != want {
		t.Errorf("vertical border is %.2f mm, want %.2f mm", got, want)
	}
	if got, want := imageWidth/imageHeight, 1600.0/900.0; math.Abs(got-want) > 0.001 {
		t.Errorf("image aspect ratio is %.3f, want %.3f", got, want)
	}
}

func TestReadRecipeImageRejectsPathsOutsideUploads(t *testing.T) {
	for _, imageURL := range []string{
		"../secrets.png",
		"/upload/../secrets.png",
		"https://example.com/image.png",
	} {
		if _, _, err := readRecipeImage(imageURL); err == nil {
			t.Errorf("expected image path %q to be rejected", imageURL)
		}
	}
}
