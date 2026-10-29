Build a modern intranet cooking website with two modes: Home (List grid recepies ), View recipes and Add/Edit recipes.
Frontend: TailwindCSS, dark modern design, drag‑and‑drop image upload, recipe image preview, stored in /www/upload/.
Backend: Go server with JSON API for CRUD and image upload.
Database: SQLite in /db/recipes.db.
Folder structure:

/www/ → HTML, CSS, JS, images

/www/upload/ → uploaded recipe images

/db/ → SQLite file

Go server in project root
Required recipe fields: title, slug, category, prep_time, cook_time, servings, ingredients, steps, image_url.
Output should follow this architecture exactly.

Path: D:\Downloads\codeRepo\WebDev\Cooking>