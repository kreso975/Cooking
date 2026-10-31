## v0.0.4 - Ongoing
- Cropped uploaded recipe images to 4:3, optimized them as JPEGs, and assigned unique timestamp-based filenames.
- Embedded the default placeholder image in single-recipe and cookbook PDFs when a recipe has no image.

## v0.0.3
- Added a footer link to export all recipes as one alphabetically ordered cookbook PDF with a cover page.
- Added a clickable table of contents and recipe bookmarks to cookbook PDFs.
- Added title, author, subject, keyword, and Creator metadata to recipe and cookbook PDFs; cookbook keywords use distinct, space-delimited recipe categories.
- Displayed recipe ingredients in columns of six, with up to three columns per page and multi-page continuation for long lists.
- Corrected EXIF orientation for JPEG images when embedding them in PDFs.

## v0.0.2
- Added recipe PDF generation and a download endpoint in the Go server.
- Added PDF download links to recipe cards and the recipe detail view.
- Styled PDFs with Home Cook branding, a compact recipe title banner, recipe image, and prep time, cook time, and servings cards.
- Added recipe ingredients and numbered instructions with multi-page support.
- Preserved recipe image proportions with a narrow, even frame and added a placeholder when no image is available.
- Set PDFs to open at actual size (100%) when supported by the PDF reader.
- Added tests for PDF downloads, image handling, page generation, layout, and invalid image paths.


## v0.0.1
- Initial release.