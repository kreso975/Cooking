package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
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

func downloadCookbookPDF(w http.ResponseWriter, r *http.Request) {
	recipes, err := loadRecipesForCookbook(r.Context())
	if err != nil {
		log.Printf("Error loading recipes for cookbook PDF: %v", err)
		http.Error(w, "Unable to load recipes", http.StatusInternalServerError)
		return
	}

	pdfBytes, err := generateCookbookPDF(recipes)
	if err != nil {
		log.Printf("Error generating cookbook PDF: %v", err)
		http.Error(w, "Unable to generate cookbook PDF", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="home-cook-recipe-book.pdf"`)
	w.Header().Set("Content-Length", fmt.Sprint(len(pdfBytes)))
	w.Header().Set("Cache-Control", "private, no-store")
	if _, err := w.Write(pdfBytes); err != nil {
		log.Printf("Error writing cookbook PDF: %v", err)
	}
}

func loadRecipesForCookbook(ctx context.Context) ([]Recipe, error) {
	rows, err := db.QueryContext(
		ctx,
		"SELECT id, title, slug, category, prep_time, cook_time, servings, ingredients, steps, image_url FROM recipes ORDER BY title COLLATE NOCASE, slug COLLATE NOCASE, id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recipes := make([]Recipe, 0)
	for rows.Next() {
		var recipe Recipe
		if err := rows.Scan(
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
		); err != nil {
			return nil, err
		}
		recipes = append(recipes, recipe)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return recipes, nil
}

func generateRecipePDF(recipe Recipe) ([]byte, error) {
	pdf := newRecipePDF()
	pdf.AddPage()
	if err := renderRecipePDFPage(pdf, recipe); err != nil {
		return nil, err
	}
	return encodeRecipePDF(pdf)
}

func generateCookbookPDF(recipes []Recipe) ([]byte, error) {
	pdf := newRecipePDF()
	pdf.AddPage()
	writePDFCover(pdf, len(recipes))
	for _, recipe := range recipes {
		pdf.AddPage()
		if err := renderRecipePDFPage(pdf, recipe); err != nil {
			return nil, fmt.Errorf("render recipe %q: %w", recipe.ID, err)
		}
	}
	return encodeRecipePDF(pdf)
}

func newRecipePDF() *gofpdf.Fpdf {
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
	return pdf
}

func writePDFCover(pdf *gofpdf.Fpdf, recipeCount int) {
	drawPDFBrand(pdf, 16, 8)
	pdf.SetFont("GoSans", "B", 30)
	pdf.SetTextColor(39, 39, 42)
	pdf.SetXY(16, 105)
	pdf.CellFormat(178, 14, "Recipe Book", "", 0, "C", false, 0, "")

	pdf.SetFont("GoSans", "", 13)
	pdf.SetTextColor(113, 113, 122)
	pdf.SetXY(16, 124)
	pdf.CellFormat(178, 8, "Home Cook recipes, collected in one place", "", 0, "C", false, 0, "")

	pdf.SetDrawColor(249, 115, 22)
	pdf.SetLineWidth(1)
	pdf.Line(76, 139, 134, 139)
	pdf.SetFont("GoSans", "B", 10)
	pdf.SetTextColor(234, 88, 12)
	pdf.SetXY(16, 146)
	pdf.CellFormat(178, 7, fmt.Sprintf("%d RECIPES", recipeCount), "", 0, "C", false, 0, "")
	if recipeCount == 0 {
		pdf.SetFont("GoSans", "", 10)
		pdf.SetTextColor(113, 113, 122)
		pdf.SetXY(16, 158)
		pdf.CellFormat(178, 7, "No recipes available yet", "", 0, "C", false, 0, "")
	}
}

func renderRecipePDFPage(pdf *gofpdf.Fpdf, recipe Recipe) error {
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
		return err
	}
	if err := writePDFOverview(pdf, recipe, imageBytes, imageFormat, contentWidth); err != nil {
		return err
	}

	writePDFSection(pdf, "Ingredients", recipe.Ingredients, true)
	writePDFSection(pdf, "Instructions", recipe.Steps, false)
	if err := pdf.Error(); err != nil {
		return fmt.Errorf("render recipe content: %w", err)
	}
	return nil
}

func encodeRecipePDF(pdf *gofpdf.Fpdf) ([]byte, error) {
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
	imageName := fmt.Sprintf("recipe-image-%d", pdf.PageNo())
	if len(imageBytes) > 0 {
		imageOptions := gofpdf.ImageOptions{ImageType: imageFormat}
		imageInfo = pdf.RegisterImageOptionsReader(imageName, imageOptions, bytes.NewReader(imageBytes))
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
		pdf.ImageOptions(imageName, imageX+imagePadding, imageY+imagePadding, renderWidth, renderHeight, false, imageOptions, 0, "")
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
	case "JPEG":
		normalized, err := normalizeJPEGOrientation(imageBytes)
		if err != nil {
			return nil, "", fmt.Errorf("normalize recipe image orientation: %w", err)
		}
		return normalized, "JPEG", nil
	case "PNG", "GIF":
		return imageBytes, strings.ToUpper(format), nil
	default:
		return nil, "", fmt.Errorf("unsupported recipe image format %q", format)
	}
}

func normalizeJPEGOrientation(data []byte) ([]byte, error) {
	orientation, err := jpegExifOrientation(data)
	if err != nil {
		return nil, err
	}
	if orientation == 1 {
		return data, nil
	}

	source, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	destinationWidth, destinationHeight := width, height
	if orientation >= 5 {
		destinationWidth, destinationHeight = height, width
	}
	destination := image.NewRGBA(image.Rect(0, 0, destinationWidth, destinationHeight))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var destinationX, destinationY int
			switch orientation {
			case 2:
				destinationX, destinationY = width-1-x, y
			case 3:
				destinationX, destinationY = width-1-x, height-1-y
			case 4:
				destinationX, destinationY = x, height-1-y
			case 5:
				destinationX, destinationY = y, x
			case 6:
				destinationX, destinationY = height-1-y, x
			case 7:
				destinationX, destinationY = height-1-y, width-1-x
			case 8:
				destinationX, destinationY = y, width-1-x
			}
			destination.Set(destinationX, destinationY, source.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}

	var normalized bytes.Buffer
	if err := jpeg.Encode(&normalized, destination, &jpeg.Options{Quality: 95}); err != nil {
		return nil, fmt.Errorf("encode normalized JPEG: %w", err)
	}
	return normalized.Bytes(), nil
}

func jpegExifOrientation(data []byte) (int, error) {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return 1, nil
	}

	for offset := 2; offset+1 < len(data); {
		if data[offset] != 0xff {
			offset++
			continue
		}
		for offset < len(data) && data[offset] == 0xff {
			offset++
		}
		if offset >= len(data) {
			break
		}
		marker := data[offset]
		offset++
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if offset+2 > len(data) {
			return 1, fmt.Errorf("truncated JPEG segment length")
		}
		segmentLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if segmentLength < 2 || offset+segmentLength > len(data) {
			return 1, fmt.Errorf("invalid JPEG segment length")
		}
		segment := data[offset+2 : offset+segmentLength]
		if marker == 0xe1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			orientation, err := parseTIFFOrientation(segment[6:])
			if err != nil {
				return 1, err
			}
			return orientation, nil
		}
		offset += segmentLength
	}
	return 1, nil
}

func parseTIFFOrientation(data []byte) (int, error) {
	if len(data) < 8 {
		return 1, fmt.Errorf("truncated EXIF TIFF header")
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1, fmt.Errorf("invalid EXIF byte order")
	}
	if order.Uint16(data[2:4]) != 42 {
		return 1, fmt.Errorf("invalid EXIF TIFF marker")
	}
	ifdOffset := uint64(order.Uint32(data[4:8]))
	if ifdOffset+2 > uint64(len(data)) {
		return 1, fmt.Errorf("EXIF IFD offset is out of bounds")
	}
	entryCount := int(order.Uint16(data[ifdOffset : ifdOffset+2]))
	entriesOffset := ifdOffset + 2
	if uint64(entryCount)*12 > uint64(len(data))-entriesOffset {
		return 1, fmt.Errorf("truncated EXIF IFD entries")
	}
	for i := 0; i < entryCount; i++ {
		entryOffset := entriesOffset + uint64(i*12)
		entry := data[entryOffset : entryOffset+12]
		if order.Uint16(entry[:2]) != 0x0112 {
			continue
		}
		if order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) < 1 {
			return 1, fmt.Errorf("invalid EXIF orientation entry")
		}
		orientation := int(order.Uint16(entry[8:10]))
		if orientation < 1 || orientation > 8 {
			return 1, fmt.Errorf("EXIF orientation %d is out of range", orientation)
		}
		return orientation, nil
	}
	return 1, nil
}
