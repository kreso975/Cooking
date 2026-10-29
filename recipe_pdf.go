package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func downloadRecipePDF(w http.ResponseWriter, r *http.Request) {
	var recipe Recipe
	err := db.QueryRowContext(
		r.Context(),
		"SELECT id, title, slug, category, prep_time, cook_time, servings, ingredients, steps, image_url FROM recipes WHERE id = ?",
		chi.URLParam(r, "id"),
	).Scan(
		&recipe.ID,
		&recipe.Title,
		&recipe.Slug,
		&recipe.Category,
		&recipe.PrepTime,
		&recipe.CookTime,
		&recipe.Servings,
		&recipe.Ingredients,
		&recipe.Steps,
		&recipe.ImageURL,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Recipe not found", http.StatusNotFound)
			return
		}
		log.Printf("Error loading recipe for PDF: %v", err)
		http.Error(w, "Unable to load recipe", http.StatusInternalServerError)
		return
	}

	pdfBytes, err := generateRecipePDF(recipe)
	if err != nil {
		log.Printf("Error generating recipe PDF for %q: %v", recipe.ID, err)
		http.Error(w, "Unable to generate recipe PDF", http.StatusInternalServerError)
		return
	}

	filename := recipe.Slug
	if !recipeSlugPattern.MatchString(filename) {
		filename = "recipe"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.pdf"`)
	w.Header().Set("Content-Length", fmt.Sprint(len(pdfBytes)))
	w.Header().Set("Cache-Control", "private, no-store")
	if _, err := w.Write(pdfBytes); err != nil {
		log.Printf("Error writing recipe PDF for %q: %v", recipe.ID, err)
	}
}

func generateRecipePDF(recipe Recipe) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetDisplayMode("real", "continuous")
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AddUTF8FontFromBytes("GoSans", "", goregular.TTF)
	pdf.AddUTF8FontFromBytes("GoSans", "B", gobold.TTF)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetDrawColor(249, 115, 22)
		pdf.SetLineWidth(0.4)
		pdf.Line(16, pdf.GetY(), 194, pdf.GetY())
		pdf.SetFont("GoSans", "", 8)
		pdf.SetTextColor(113, 113, 122)
		pdf.CellFormat(0, 6, fmt.Sprintf("HOME COOK  ·  PAGE %d", pdf.PageNo()), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()

	drawPDFBrand(pdf, 16, 8)

	const contentWidth = 178.0
	headerX, headerY := 16.0, 24.0
	titleFontSize, titleLineHeight := 21.0, 9.0
	pdf.SetFont("GoSans", "B", titleFontSize)
	titleLines := pdf.SplitText(recipe.Title, contentWidth-16)
	headerHeight := 4 + float64(len(titleLines))*titleLineHeight + 4
	pdf.SetFillColor(24, 24, 27)
	pdf.Rect(headerX, headerY, contentWidth, headerHeight, "F")
	pdf.SetXY(headerX+8, headerY+2)
	pdf.SetFont("GoSans", "B", titleFontSize)
	pdf.SetTextColor(255, 255, 255)
	pdf.MultiCell(contentWidth-16, titleLineHeight, recipe.Title, "", "L", false)
	pdf.SetY(headerY + headerHeight + 6)

	imageBytes, imageFormat, err := readRecipeImage(recipe.ImageURL)
	if err != nil {
		return nil, err
	}
	if err := writePDFOverview(pdf, recipe, imageBytes, imageFormat, contentWidth); err != nil {
		return nil, err
	}

	writePDFSection(pdf, "Ingredients", recipe.Ingredients, true)
	writePDFSection(pdf, "Instructions", recipe.Steps, false)
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("render recipe content: %w", err)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, fmt.Errorf("encode PDF: %w", err)
	}
	return output.Bytes(), nil
}

func writePDFOverview(pdf *gofpdf.Fpdf, recipe Recipe, imageBytes []byte, imageFormat string, contentWidth float64) error {
	const (
		maxImageWidth  = 104.0
		maxImageHeight = 64.0
		imagePadding   = 1.0
		gap            = 6.0
	)
	frameWidth, frameHeight := maxImageWidth, maxImageHeight
	imageInfo := (*gofpdf.ImageInfoType)(nil)
	if len(imageBytes) > 0 {
		imageOptions := gofpdf.ImageOptions{ImageType: imageFormat}
		imageInfo = pdf.RegisterImageOptionsReader("recipe-image", imageOptions, bytes.NewReader(imageBytes))
		if imageInfo == nil {
			return fmt.Errorf("register recipe image: %w", pdf.Error())
		}
		frameWidth, frameHeight, _, _ = fitPDFImage(imageInfo.Width(), imageInfo.Height(), maxImageWidth, maxImageHeight, imagePadding)
	}

	imageX, imageY := 16.0, pdf.GetY()
	pdf.SetFillColor(244, 244, 245)
	pdf.SetDrawColor(212, 212, 216)
	pdf.SetLineWidth(0.25)
	pdf.Rect(imageX, imageY, frameWidth, frameHeight, "DF")
	if len(imageBytes) > 0 {
		imageOptions := gofpdf.ImageOptions{ImageType: imageFormat}
		_, _, renderWidth, renderHeight := fitPDFImage(imageInfo.Width(), imageInfo.Height(), maxImageWidth, maxImageHeight, imagePadding)
		pdf.ImageOptions("recipe-image", imageX+imagePadding, imageY+imagePadding, renderWidth, renderHeight, false, imageOptions, 0, "")
		if err := pdf.Error(); err != nil {
			return fmt.Errorf("add recipe image: %w", err)
		}
	} else {
		pdf.SetFont("GoSans", "", 9)
		pdf.SetTextColor(113, 113, 122)
		pdf.SetXY(imageX, imageY+frameHeight/2-3)
		pdf.CellFormat(frameWidth, 6, "No recipe image", "", 0, "C", false, 0, "")
	}

	metadataX := imageX + frameWidth + gap
	metadataWidth := contentWidth - frameWidth - gap
	writePDFMetadata(pdf, metadataX, imageY, metadataWidth, frameHeight, recipe)
	pdf.SetY(imageY + frameHeight + 6)
	return nil
}

func fitPDFImage(imageWidth, imageHeight, maxWidth, maxHeight, padding float64) (frameWidth, frameHeight, renderWidth, renderHeight float64) {
	renderWidth, renderHeight = maxWidth-2*padding, maxHeight-2*padding
	if scale := math.Min(renderWidth/imageWidth, renderHeight/imageHeight); scale > 0 {
		renderWidth = imageWidth * scale
		renderHeight = imageHeight * scale
	}
	return renderWidth + 2*padding, renderHeight + 2*padding, renderWidth, renderHeight
}

func drawPDFBrand(pdf *gofpdf.Fpdf, x, y float64) {
	pdf.SetLineWidth(0.9)
	pdf.SetDrawColor(147, 51, 234)
	pdf.SetFillColor(126, 34, 206)
	pdf.Circle(x+3.5, y+3.5, 3, "DF")
	pdf.SetLineWidth(1.6)
	pdf.Line(x+5.6, y+5.6, x+8.5, y+8.5)
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(255, 255, 255)
	pdf.Circle(x+3.5, y+3.5, 1.6, "F")
	pdf.SetFillColor(251, 191, 36)
	pdf.SetDrawColor(251, 191, 36)
	pdf.Circle(x+3.5, y+3.5, 0.8, "F")

	pdf.SetFont("GoSans", "B", 16)
	pdf.SetTextColor(249, 115, 22)
	pdf.SetXY(x+12, y+1)
	pdf.CellFormat(0, 10, "Home Cook", "", 0, "L", false, 0, "")
	pdf.SetDrawColor(228, 228, 231)
	pdf.SetLineWidth(0.3)
	pdf.Line(16, y+14, 194, y+14)
}

func writePDFMetadata(pdf *gofpdf.Fpdf, x, y, width, height float64, recipe Recipe) {
	values := []struct {
		label string
		value string
	}{
		{label: "PREP TIME", value: recipe.PrepTime},
		{label: "COOK TIME", value: recipe.CookTime},
		{label: "SERVINGS", value: fmt.Sprint(recipe.Servings)},
	}
	const cardGap = 2.5
	cardHeight := (height - float64(len(values)-1)*cardGap) / float64(len(values))
	for i, item := range values {
		cardY := y + float64(i)*(cardHeight+cardGap)
		pdf.SetFillColor(244, 244, 245)
		pdf.SetDrawColor(212, 212, 216)
		pdf.SetLineWidth(0.25)
		pdf.Rect(x, cardY, width, cardHeight, "DF")
		textTop := cardY + (cardHeight-9)/2
		pdf.SetXY(x+4, textTop)
		pdf.SetFont("GoSans", "B", 7)
		pdf.SetTextColor(249, 115, 22)
		pdf.CellFormat(width-8, 4, item.label, "", 0, "L", false, 0, "")
		pdf.SetXY(x+4, textTop+4)
		pdf.SetFont("GoSans", "B", 10)
		pdf.SetTextColor(39, 39, 42)
		pdf.MultiCell(width-8, 5, item.value, "", "L", false)
	}
}

func writePDFSection(pdf *gofpdf.Fpdf, heading, content string, useBullets bool) {
	pdf.SetFont("GoSans", "B", 15)
	pdf.SetTextColor(234, 88, 12)
	pdf.Ln(2)
	pdf.MultiCell(0, 8, heading, "", "L", false)
	pdf.SetDrawColor(228, 228, 231)
	pdf.SetLineWidth(0.3)
	pdf.Line(16, pdf.GetY(), 194, pdf.GetY())
	pdf.Ln(3)

	pdf.SetFont("GoSans", "", 11)
	pdf.SetTextColor(63, 63, 70)
	lineNumber := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		prefix := ""
		if useBullets {
			prefix = "- "
		} else {
			lineNumber++
			prefix = fmt.Sprintf("%d. ", lineNumber)
		}
		pdf.MultiCell(0, 6, prefix+line, "", "L", false)
		pdf.Ln(1)
	}
}

func readRecipeImage(imageURL string) ([]byte, string, error) {
	if imageURL == "" {
		return nil, "", nil
	}
	uploadPath := strings.TrimPrefix(imageURL, "/www")
	if !strings.HasPrefix(uploadPath, "/upload/") {
		return nil, "", fmt.Errorf("unsupported recipe image path")
	}

	filename := strings.TrimPrefix(uploadPath, "/upload/")
	if filename == "" || filename == "." || filename == ".." || strings.ContainsAny(filename, `/\`) {
		return nil, "", fmt.Errorf("invalid recipe image path")
	}
	imagePath := filepath.Join(webRoot, "upload", filename)
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, "", fmt.Errorf("read recipe image: %w", err)
	}

	_, format, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, "", fmt.Errorf("decode recipe image: %w", err)
	}
	switch strings.ToUpper(format) {
	case "JPEG", "PNG", "GIF":
		return imageBytes, strings.ToUpper(format), nil
	default:
		return nil, "", fmt.Errorf("unsupported recipe image format %q", format)
	}
}
