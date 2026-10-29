// $env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"; go build -ldflags="-s -w" -trimpath -o app/homeCookingServer .

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Recipe struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	Category    string `json:"category"`
	PrepTime    string `json:"prep_time"`
	CookTime    string `json:"cook_time"`
	Servings    int    `json:"servings"`
	Ingredients string `json:"ingredients"`
	Steps       string `json:"steps"`
	ImageURL    string `json:"image_url"`
}

var db *sql.DB
var webRoot string
var recipeSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func main() {
	port, err := getPort()
	if err != nil {
		log.Fatal(err)
	}

	dnsServer := envOrDefault("DNS", "192.168.1.53")
	if err := configureDNS(dnsServer); err != nil {
		log.Fatal(err)
	}

	webRoot, err = getWebRoot()
	if err != nil {
		log.Fatal(err)
	}
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		log.Fatal(err)
	}

	db, err = sql.Open("sqlite", "./db/recipes.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	createTable()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	fs := http.FileServer(http.Dir(webRoot))
	r.Get("/", fs.ServeHTTP)
	r.Get("/recipes/{slug}", serveRecipePage)
	r.NotFound(fs.ServeHTTP)
	r.Handle("/www/*", http.StripPrefix("/www", fs))

	r.Get("/api/recipes", getRecipes)
	r.Get("/api/recipes/slug/{slug}", getRecipeBySlug)
	r.Get("/api/recipes/{id}/pdf", downloadRecipePDF)
	r.Get("/api/recipes/{id}", getRecipe)
	r.Post("/api/recipes", createRecipe)
	r.Put("/api/recipes/{id}", updateRecipe)
	r.Delete("/api/recipes/{id}", deleteRecipe)

	r.Post("/api/upload", uploadImage)

	address := net.JoinHostPort("", strconv.Itoa(port))
	log.Printf("Server starting at %s (DNS %s, web root %s)", address, dnsServer, webRoot)
	log.Fatal(http.ListenAndServe(address, r))
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func getWebRoot() (string, error) {
	if configuredRoot := os.Getenv("WWW_DIR"); configuredRoot != "" {
		if err := validateWebRoot(configuredRoot); err != nil {
			return "", fmt.Errorf("invalid WWW_DIR: %w", err)
		}
		return configuredRoot, nil
	}

	for _, root := range []string{"/www", "./www"} {
		if err := validateWebRoot(root); err == nil {
			return root, nil
		}
	}
	return "", fmt.Errorf("index.html not found in /www or ./www; set WWW_DIR to the directory containing index.html")
}

func validateWebRoot(root string) error {
	info, err := os.Stat(filepath.Join(root, "index.html"))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s/index.html is not a regular file", root)
	}
	return nil
}

func getPort() (int, error) {
	value := envOrDefault("PORT", "8080")
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid PORT %q: must be an integer between 1 and 65535", value)
	}
	return port, nil
}

func configureDNS(server string) error {
	host, port, err := net.SplitHostPort(server)
	if err != nil {
		if net.ParseIP(server) == nil {
			return fmt.Errorf("invalid DNS %q: expected an IP address, optionally with a port", server)
		}
		host, port = server, "53"
	}
	if net.ParseIP(host) == nil {
		return fmt.Errorf("invalid DNS %q: server must be an IP address", server)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("invalid DNS %q: port must be an integer between 1 and 65535", server)
	}

	address := net.JoinHostPort(host, strconv.Itoa(portNumber))
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
	return nil
}

func createTable() {
	query := `
	CREATE TABLE IF NOT EXISTS recipes (
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
	);`
	_, err := db.Exec(query)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_recipes_slug ON recipes(slug)")
	if err != nil {
		log.Fatal(err)
	}
}

func serveRecipePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
}

func getRecipes(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, title, slug, category, prep_time, cook_time, servings, ingredients, steps, image_url FROM recipes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	recipes := []Recipe{}
	for rows.Next() {
		var rcp Recipe
		rows.Scan(&rcp.ID, &rcp.Title, &rcp.Slug, &rcp.Category, &rcp.PrepTime, &rcp.CookTime, &rcp.Servings, &rcp.Ingredients, &rcp.Steps, &rcp.ImageURL)
		recipes = append(recipes, rcp)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recipes)
}

func getRecipe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var rcp Recipe
	err := db.QueryRow("SELECT id, title, slug, category, prep_time, cook_time, servings, ingredients, steps, image_url FROM recipes WHERE id = ?", id).
		Scan(&rcp.ID, &rcp.Title, &rcp.Slug, &rcp.Category, &rcp.PrepTime, &rcp.CookTime, &rcp.Servings, &rcp.Ingredients, &rcp.Steps, &rcp.ImageURL)

	if err != nil {
		http.Error(w, "Recipe not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rcp)
}

func getRecipeBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var rcp Recipe
	err := db.QueryRow("SELECT id, title, slug, category, prep_time, cook_time, servings, ingredients, steps, image_url FROM recipes WHERE slug = ?", slug).
		Scan(&rcp.ID, &rcp.Title, &rcp.Slug, &rcp.Category, &rcp.PrepTime, &rcp.CookTime, &rcp.Servings, &rcp.Ingredients, &rcp.Steps, &rcp.ImageURL)

	if err != nil {
		http.Error(w, "Recipe not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rcp)
}

func recipeSlugExists(slug, excludeID string) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM recipes WHERE slug = ? AND id <> ?", slug, excludeID).Scan(&count)
	return count > 0, err
}

func createRecipe(w http.ResponseWriter, r *http.Request) {
	var rcp Recipe
	if err := json.NewDecoder(r.Body).Decode(&rcp); err != nil {
		log.Printf("JSON Decode Error (Create): %v", err) // Added logging
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !recipeSlugPattern.MatchString(rcp.Slug) {
		http.Error(w, "Slug must contain lowercase letters, numbers, and single hyphens only", http.StatusBadRequest)
		return
	}
	exists, err := recipeSlugExists(rcp.Slug, "")
	if err != nil {
		http.Error(w, "Unable to validate recipe slug", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "A recipe with this slug already exists", http.StatusConflict)
		return
	}

	rcp.ID = uuid.New().String()
	_, err = db.Exec("INSERT INTO recipes (id, title, slug, category, prep_time, cook_time, servings, ingredients, steps, image_url) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		rcp.ID, rcp.Title, rcp.Slug, rcp.Category, rcp.PrepTime, rcp.CookTime, rcp.Servings, rcp.Ingredients, rcp.Steps, rcp.ImageURL)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rcp)
}

func updateRecipe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var rcp Recipe
	if err := json.NewDecoder(r.Body).Decode(&rcp); err != nil {
		log.Printf("JSON Decode Error (Update): %v", err) // Added logging
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !recipeSlugPattern.MatchString(rcp.Slug) {
		http.Error(w, "Slug must contain lowercase letters, numbers, and single hyphens only", http.StatusBadRequest)
		return
	}
	rcp.ID = id
	exists, err := recipeSlugExists(rcp.Slug, id)
	if err != nil {
		http.Error(w, "Unable to validate recipe slug", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "A recipe with this slug already exists", http.StatusConflict)
		return
	}

	_, err = db.Exec("UPDATE recipes SET title=?, slug=?, category=?, prep_time=?, cook_time=?, servings=?, ingredients=?, steps=?, image_url=? WHERE id=?",
		rcp.Title, rcp.Slug, rcp.Category, rcp.PrepTime, rcp.CookTime, rcp.Servings, rcp.Ingredients, rcp.Steps, rcp.ImageURL, rcp.ID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteRecipe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := db.Exec("DELETE FROM recipes WHERE id=?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func uploadImage(w http.ResponseWriter, r *http.Request) {
	file, handler, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Error retrieving the file", http.StatusBadRequest)
		return
	}
	defer func() {
		_ = file.Close()
	}()

	filename := handler.Filename
	filename = filepath.Base(filename)
	if filename == "." || filename == string(filepath.Separator) {
		http.Error(w, "Invalid file name", http.StatusBadRequest)
		return
	}

	uploadDir := filepath.Join(webRoot, "upload")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		http.Error(w, "Error creating upload directory", http.StatusInternalServerError)
		return
	}
	dstPath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "Error saving the file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Error copying the file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"url": "/upload/" + filename})
}
