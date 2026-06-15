package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/template/html/v2"

	"github.com/rgunasekaran-github/ephemeral-demo/internal/store"
)

const version = "0.1.1"

func main() {
	ctx := context.Background()

	connStr := os.Getenv("COSMOS_CONNECTION_STRING")
	endpoint := getenv("COSMOS_ENDPOINT", "https://localhost:8081")
	key := getenv("COSMOS_KEY", "")
	dbName := getenv("COSMOS_DB", "tododb")
	container := getenv("COSMOS_CONTAINER", "todos")
	insecure := getenv("COSMOS_INSECURE", "false") == "true"

	var httpClient *http.Client
	if insecure {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 30 * time.Second,
		}
	}

	todoStore, err := store.New(ctx, store.Config{
		ConnectionString: connStr,
		Endpoint:         endpoint,
		Key:              key,
		Database:         dbName,
		Container:        container,
		HTTPClient:       httpClient,
	})
	if err != nil {
		log.Fatalf("cosmos init: %v", err)
	}

	engine := html.New("./views", ".html")
	app := fiber.New(fiber.Config{Views: engine})
	app.Use(logger.New())

	prNumber := getenv("PR_NUMBER", "local")
	branchName := getenv("BRANCH_NAME", "local")

	app.Get("/", func(c fiber.Ctx) error {
		todos, err := todoStore.List(c.Context())
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Render("index", fiber.Map{
			"Todos":  todos,
			"PR":     prNumber,
			"Branch": branchName,
			"DB":     dbName,
		})
	})

	app.Post("/todos", func(c fiber.Ctx) error {
		title := c.FormValue("title")
		if title == "" {
			return c.Redirect().Status(fiber.StatusSeeOther).To("/")
		}
		if _, err := todoStore.Create(c.Context(), title); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Redirect().Status(fiber.StatusSeeOther).To("/")
	})

	app.Post("/todos/:id/toggle", func(c fiber.Ctx) error {
		if err := todoStore.Toggle(c.Context(), c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Redirect().Status(fiber.StatusSeeOther).To("/")
	})

	app.Post("/todos/:id/delete", func(c fiber.Ctx) error {
		if err := todoStore.Delete(c.Context(), c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Redirect().Status(fiber.StatusSeeOther).To("/")
	})

	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok") })

	log.Fatal(app.Listen(":" + getenv("PORT", "3000")))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
