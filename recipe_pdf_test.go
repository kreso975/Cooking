package main

import (
	"bytes"
	"database/sql"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/go-chi/chi/v5"
)

func setupRecipePDFTest(t *testing.T, recipes ...Recipe) http.Handler {
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
	for _, recipe := range recipes {
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
	router.Get("/api/cookbook.pdf", downloadCookbookPDF)
	return router
}

func sampleRecipeWithTitle(id, title string) Recipe {
	recipe := samplePDFRecipe()
	recipe.ID = id
	recipe.Title = title
	recipe.Slug = strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	return recipe
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

func requestCookbookPDF(router http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/cookbook.pdf", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestDownloadCookbookPDF(t *testing.T) {
	dessert := sampleRecipeWithTitle("recipe-z", "Zucchini Soup")
	dessert.Category = `"Dessert"`
	mainCourse := sampleRecipeWithTitle("recipe-a", "Apple Pie")
	mainCourse.Category = `"Main Course"`
	router := setupRecipePDFTest(
		t,
		dessert,
		mainCourse,
	)
	response := requestCookbookPDF(router)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("expected application/pdf, got %q", got)
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="home-cook-recipe-book.pdf"` {
		t.Fatalf("unexpected Content-Disposition: %q", got)
	}
	if !bytes.HasPrefix(response.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("response does not contain a PDF")
	}
	assertPDFMetadata(t, response.Body.Bytes(), "Home Cook Recipe Book", "Home Cook", "Recipe", "Dessert Main Course")
	if pages := bytes.Count(response.Body.Bytes(), []byte("/Type /Page")); pages < 5 {
		t.Fatalf("expected a cover, contents page, and two recipe pages, found at least %d pages", pages-1)
	}
	if links := bytes.Count(response.Body.Bytes(), []byte("/Subtype /Link")); links != 2 {
		t.Fatalf("expected two clickable contents entries, found %d", links)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("/Outlines ")) ||
		!bytes.Contains(response.Body.Bytes(), []byte("/PageMode /UseOutlines")) {
		t.Fatal("PDF does not expose recipe bookmarks in the document outline")
	}

	recipes, err := loadRecipesForCookbook(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) != 2 || recipes[0].Title != "Apple Pie" || recipes[1].Title != "Zucchini Soup" {
		t.Fatalf("recipes are not sorted alphabetically: %+v", recipes)
	}
}

func TestDownloadEmptyCookbookPDF(t *testing.T) {
	router := setupRecipePDFTest(t)
	response := requestCookbookPDF(router)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if !bytes.HasPrefix(response.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("empty cookbook response does not contain a PDF")
	}
	if pages := bytes.Count(response.Body.Bytes(), []byte("/Type /Page")); pages != 2 {
		t.Fatalf("expected a single cover page, found %d pages", pages-1)
	}
}

func TestDownloadRecipePDF(t *testing.T) {
	recipe := samplePDFRecipe()
	recipe.Category = `"Dessert"`
	router := setupRecipePDFTest(t, recipe)
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
	assertPDFMetadata(t, response.Body.Bytes(), "Čokoladni kolač", "Home Cook", "Recipe", "Dessert")
	if !bytes.HasSuffix(bytes.TrimSpace(response.Body.Bytes()), []byte("%%EOF")) {
		t.Fatal("PDF does not have an EOF marker")
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("/XYZ null null 1")) {
		t.Fatal("PDF does not request actual-size initial zoom")
	}
}

func TestCookbookCategoryKeywordsAreDistinctAndSorted(t *testing.T) {
	keywords := cookbookCategoryKeywords([]Recipe{
		{Category: `"Dessert"`},
		{Category: `"Main Course"`},
		{Category: " dessert "},
		{Category: ""},
	})
	if keywords != "Dessert Main Course" {
		t.Fatalf("unexpected cookbook category keywords %q", keywords)
	}
}

func TestRecipePDFMetadataDoesNotQuoteKeywordValue(t *testing.T) {
	pdf := newRecipePDF()
	setRecipePDFMetadata(pdf, "Recipe Book", "Blitva Hrenovka Piletina Poriluk Riba")
	pdf.AddPage()

	pdfBytes, err := encodeRecipePDF(pdf)
	if err != nil {
		t.Fatal(err)
	}
	assertPDFMetadata(t, pdfBytes, "Recipe Book", "Home Cook", "Recipe", "Blitva Hrenovka Piletina Poriluk Riba")
	if bytes.Contains(pdfBytes, []byte(`"Blitva Hrenovka Piletina Poriluk Riba"`)) {
		t.Fatal("PDF metadata wraps keywords in quotes")
	}
}

func assertPDFMetadata(t *testing.T, pdfBytes []byte, title, author, subject, keywords string) {
	t.Helper()

	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "Title", value: title},
		{name: "Author", value: author},
		{name: "Subject", value: subject},
		{name: "Keywords", value: keywords},
	} {
		encodedValue := utf16PDFString(field.value)
		encodedValue = bytes.ReplaceAll(encodedValue, []byte(`\`), []byte(`\\`))
		encodedValue = bytes.ReplaceAll(encodedValue, []byte("("), []byte(`\(`))
		encodedValue = bytes.ReplaceAll(encodedValue, []byte(")"), []byte(`\)`))
		encodedValue = bytes.ReplaceAll(encodedValue, []byte("\r"), []byte(`\r`))
		expected := append([]byte("/"+field.name+" ("), encodedValue...)
		expected = append(expected, ')')
		if !bytes.Contains(pdfBytes, expected) {
			start := bytes.Index(pdfBytes, []byte("/"+field.name+" "))
			end := min(len(pdfBytes), start+160)
			t.Errorf("PDF metadata %s does not contain %q: % x", field.name, field.value, pdfBytes[start:end])
		}
	}
}

func utf16PDFString(value string) []byte {
	encoded := []byte{0xfe, 0xff}
	for _, character := range utf16.Encode([]rune(value)) {
		encoded = append(encoded, byte(character>>8), byte(character))
	}
	return encoded
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

func TestNormalizeJPEGOrientationRotatesImage(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			source.Set(x, y, color.RGBA{R: uint8(x * 70), G: uint8(y * 100), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	withOrientation := addJPEGExifOrientation(encoded.Bytes(), 8)

	normalized, err := normalizeJPEGOrientation(withOrientation)
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(normalized))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 2 || config.Height != 3 {
		t.Fatalf("expected rotated JPEG dimensions 2x3, got %dx%d", config.Width, config.Height)
	}
	orientation, err := jpegExifOrientation(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if orientation != 1 {
		t.Fatalf("expected normalized JPEG to have default orientation, got %d", orientation)
	}
}

func addJPEGExifOrientation(jpegData []byte, orientation byte) []byte {
	tiff := []byte{
		'I', 'I', 42, 0, 8, 0, 0, 0,
		1, 0, 0x12, 0x01, 3, 0, 1, 0, 0, 0,
		orientation, 0, 0, 0,
		0, 0, 0, 0,
	}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segmentLength := uint16(len(payload) + 2)
	segment := []byte{0xff, 0xe1, byte(segmentLength >> 8), byte(segmentLength)}
	segment = append(segment, payload...)
	result := append([]byte{}, jpegData[:2]...)
	result = append(result, segment...)
	return append(result, jpegData[2:]...)
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
