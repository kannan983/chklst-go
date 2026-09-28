package main

import (
	"chklst-go/internal/api/handlers"
	"chklst-go/internal/api/middleware"
	"chklst-go/internal/auth"
	"chklst-go/internal/crypto"
	"chklst-go/internal/database"
	"chklst-go/internal/teams"
	"chklst-go/internal/utils"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/static"
)

func main() {
	// Command line flags (override environment variables)
	flagPort := flag.String("port", "", "Port to run on (default: 8000)")
	flagDB := flag.String("db", "", "Database path (default: ./chklst.db)")
	flagBackupDir := flag.String("backup-dir", "", "Backup directory (default: ./backups)")
	flag.Parse()

	// Initialize logger
	utils.InitLogger(utils.INFO)

	// Initialize at-rest secret encryption (no-op without APP_ENCRYPTION_KEY)
	crypto.Init()
	// Initialize optional auth gate (no-op unless AUTH_ENABLED=true)
	auth.Init()
	logger := utils.AppLogger

	logger.Info("Starting chklst-go application", nil)

	// Get configuration: flags override environment, which overrides defaults
	dbPath := getConfigValue(*flagDB, "DB_PATH", "./chklst.db")
	backupDir := getConfigValue(*flagBackupDir, "BACKUP_DIR", "./backups")
	port := getConfigValue(*flagPort, "PORT", "8000")
	autoBackupHours := getEnvAsInt("AUTO_BACKUP_HOURS", 24)

	logger.Info("Configuration loaded", map[string]interface{}{
		"db_path":           dbPath,
		"backup_dir":        backupDir,
		"port":              port,
		"auto_backup_hours": autoBackupHours,
	})

	// Initialize database
	logger.Info("Initializing database", map[string]interface{}{"path": dbPath})
	if err := database.InitDatabase(dbPath); err != nil {
		logger.Error("Failed to initialize database", err, map[string]interface{}{
			"db_path": dbPath,
		})
		os.Exit(1)
	}
	logger.Info("Database initialized successfully", nil)

	// Run auto-migration to create/update tables
	if err := database.AutoMigrate(); err != nil {
		logger.Error("Failed to run auto-migration", err, nil)
		os.Exit(1)
	}

	// Initialize backup manager
	logger.Info("Initializing backup manager", map[string]interface{}{"backup_dir": backupDir})
	backupManager := utils.NewBackupManager(backupDir)

	// Initialize admin handlers with backup manager
	handlers.InitAdminHandlers(backupManager)

	// Start Parson's daily-report scheduler (generate/send at configured times)
	handlers.StartScheduler()

	// Start the Teams MQTT logger (no-op unless MQTT_URL is set)
	teams.Start()

	// Start auto-backup scheduler
	logger.Info("Starting auto-backup scheduler", map[string]interface{}{
		"interval_hours": autoBackupHours,
	})
	backupManager.StartAutoBackup(dbPath, autoBackupHours)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "chklst-go v1.0.0",
		ServerHeader: "chklst-go",
		ErrorHandler: customErrorHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	})

	// Global middleware
	app.Use(middleware.RequestID())
	app.Use(middleware.RequestLogger())
	app.Use(middleware.Recovery())
	app.Use(middleware.SecurityHeaders())
	app.Use(auth.Middleware()) // gates /api/v1/* when AUTH_ENABLED
	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:8000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           86400,
	}))

	// Health check endpoint (outside API group)
	app.Get("/health", handlers.HealthCheck)

	// API v1 routes
	v1 := app.Group("/api/v1")

	// Projects routes
	projects := v1.Group("/projects")
	projects.Get("/", handlers.ListProjects)
	// Static paths MUST be registered before "/:id" or Fiber matches them as an ID
	// (e.g. GET /projects/export would otherwise hit GetProject with id="export").
	projects.Get("/export", handlers.ExportProjectsTOML)
	projects.Post("/import/preview", handlers.ImportProjectsPreview)
	projects.Post("/import", handlers.ImportProjectsTOML)
	projects.Get("/:id", handlers.GetProject)
	projects.Get("/:id/export", handlers.ExportSingleProjectTOML)
	projects.Post("/", handlers.CreateProject)
	projects.Put("/:id", handlers.UpdateProject)
	projects.Delete("/:id", handlers.DeleteProject)
	projects.Post("/:id/duplicate", handlers.DuplicateProject)

	// Components routes (nested under projects)
	projects.Post("/:projectId/components", handlers.CreateComponent)
	projects.Put("/:projectId/components/:componentId", handlers.UpdateComponent)
	projects.Delete("/:projectId/components/:componentId", handlers.DeleteComponent)

	// Deployments routes
	deployments := v1.Group("/deployments")
	deployments.Get("/", handlers.ListDeployments)
	deployments.Get("/:id", handlers.GetDeployment)
	deployments.Post("/", handlers.CreateDeployment)
	deployments.Put("/:id", handlers.UpdateDeployment)
	deployments.Delete("/:id", handlers.DeleteDeployment)

	// Library routes
	library := v1.Group("/library")
	library.Get("/", handlers.GetLibrary)
	library.Put("/", handlers.UpdateLibrary)
	library.Post("/developers", handlers.AddDeveloper)
	library.Delete("/developers/:name", handlers.RemoveDeveloper)
	library.Post("/build-servers", handlers.AddBuildServer)
	library.Delete("/build-servers/:name", handlers.RemoveBuildServer)
	library.Post("/deploy-servers", handlers.AddDeployServer)
	library.Delete("/deploy-servers/:name", handlers.RemoveDeployServer)
	library.Post("/environments", handlers.AddEnvironment)
	library.Delete("/environments/:name", handlers.RemoveEnvironment)
	library.Get("/export", handlers.ExportLibraryTOML)
	library.Post("/import", handlers.ImportLibraryTOML)

	// Auth routes (always reachable, even when the gate is on)
	authGroup := v1.Group("/auth")
	authGroup.Get("/status", handlers.AuthStatus)
	authGroup.Post("/login", handlers.Login)
	authGroup.Post("/logout", handlers.Logout)

	// Settings routes
	settings := v1.Group("/settings")
	settings.Get("/", handlers.GetSettings)
	settings.Post("/", handlers.UpdateSettings)

	// AI / Ollama routes
	aiGroup := v1.Group("/ai")
	aiGroup.Post("/test", handlers.TestAI)
	aiGroup.Get("/models", handlers.ListAIModels)
	aiGroup.Get("/defaults", handlers.GetParsonDefaults)

	// Daily summary routes
	summaryGroup := v1.Group("/summary")
	summaryGroup.Get("/today", handlers.GetTodaySummary)
	summaryGroup.Get("/recent", handlers.ListRecentSummaries)
	summaryGroup.Put("/:id", handlers.UpdateSummary)
	summaryGroup.Post("/:id/generate", handlers.GenerateSummary)
	summaryGroup.Post("/:id/send", handlers.SendSummary)

	// Email (SMTP) routes
	email := v1.Group("/email")
	email.Post("/test", handlers.TestEmail)

	// Analytics (AI deployment analysis) routes
	analytics := v1.Group("/analytics")
	analytics.Get("/insights", handlers.GetInsights)
	analytics.Get("/narrative", handlers.GetAnalyticsNarrative)

	// Git Insights (GitHub commit analytics) routes
	gitGroup := v1.Group("/git")
	gitGroup.Post("/test", handlers.TestGitHub)
	gitGroup.Get("/insights", handlers.GetGitInsights)

	// My Teams (Teams for Linux MQTT call analytics) routes
	teamsGroup := v1.Group("/teams")
	teamsGroup.Get("/report", handlers.GetTeamsReport)
	teamsGroup.Get("/live", handlers.GetTeamsLive)

	// Holiday routes
	holidays := v1.Group("/holidays")
	holidays.Get("/", handlers.ListHolidays)
	holidays.Post("/", handlers.AddHoliday)
	holidays.Delete("/:id", handlers.DeleteHoliday)

	// Jira routes
	jira := v1.Group("/jira")
	jira.Get("/tickets", handlers.GetJiraTickets)
	jira.Get("/tickets/:key", handlers.GetJiraTicket)
	jira.Post("/tickets/:key/comment", handlers.PostJiraComment)
	jira.Post("/test", handlers.TestJiraConnection)

	// Webhook routes
	webhooks := v1.Group("/webhooks")
	webhooks.Post("/test/teams", handlers.TestTeamsWebhook)

	// Admin routes
	admin := v1.Group("/admin")
	admin.Post("/backup/database", handlers.BackupDatabase)
	admin.Post("/restore/database", handlers.RestoreDatabase)
	admin.Post("/export/settings", handlers.ExportSettings)
	admin.Post("/import/settings", handlers.ImportSettings)
	admin.Get("/backups", handlers.ListBackups)

	// Serve the SPA shell (index.html) with no-cache so a freshly built bundle is
	// always picked up. Hashed assets under /assets are immutable and stay cacheable.
	// Without this, the browser keeps a stale index.html pointing at a deleted bundle
	// and the app renders blank after a rebuild.
	serveShell := func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		return c.SendFile("./frontend/dist/index.html")
	}
	app.Get("/", serveShell)

	// Static files (hashed JS/CSS, favicon, etc.)
	app.Use("/", static.New("./frontend/dist"))

	// SPA fallback - serve index.html for all non-API client routes (Vue Router)
	app.Use(func(c fiber.Ctx) error {
		if c.Path() != "/health" && !strings.HasPrefix(c.Path(), "/api") {
			return serveShell(c)
		}
		return c.Next()
	})

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logger.Info("Shutting down gracefully...", nil)

		// Close database connection
		sqlDB, err := database.DB.DB()
		if err == nil {
			sqlDB.Close()
			logger.Info("Database connection closed", nil)
		}

		// Shutdown server
		if err := app.Shutdown(); err != nil {
			logger.Error("Server shutdown error", err, nil)
		}

		logger.Info("Application stopped", nil)
		os.Exit(0)
	}()

	// Start server
	addr := fmt.Sprintf(":%s", port)
	logger.Info("Starting HTTP server", map[string]interface{}{
		"address": addr,
		"port":    port,
	})

	if err := app.Listen(addr); err != nil {
		logger.Error("Failed to start server", err, map[string]interface{}{
			"address": addr,
		})
		os.Exit(1)
	}
}

// customErrorHandler handles Fiber errors
func customErrorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
	})
}

// getEnv gets environment variable with default fallback
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt gets environment variable as int with default fallback
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getConfigValue returns flag value if set, then env value, then default
func getConfigValue(flagValue, envKey, defaultValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}
