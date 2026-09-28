package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDatabase initializes the SQLite database connection
func InitDatabase(dbPath string) error {
	var err error

	// Configure GORM logger - Silent mode for cleaner output
	// SQL queries are not logged by default; set to logger.Info if debugging needed
	gormLogger := logger.Default.LogMode(logger.Silent)

	// Open SQLite connection
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger:                 gormLogger,
		SkipDefaultTransaction: true, // Better performance
		PrepareStmt:            true, // Cache prepared statements
	})

	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("✅ Database connection established")

	return nil
}

// AutoMigrate runs auto-migration for all models
func AutoMigrate() error {
	log.Println("🔄 Running auto-migration...")

	// For existing databases, we use a safe migration approach
	// SQLite has limitations with ALTER TABLE, and GORM may try to recreate tables
	// when it detects schema differences (like VARCHAR(100) vs TEXT)

	// Only run AutoMigrate for tables that don't exist yet
	// For existing tables, we manually add missing columns

	var migrationErr error

	// Migrate new/simple tables first
	migrationErr = DB.AutoMigrate(&Library{}, &Settings{}, &DailySummary{}, &Holiday{}, &TeamsEvent{})
	if migrationErr != nil {
		return fmt.Errorf("auto-migration failed for library/settings/daily_summaries/holidays/teams_events: %w", migrationErr)
	}

	// For projects, components, deployments - use safe migration
	if !tableExists("projects") {
		migrationErr = DB.AutoMigrate(&Project{})
		if migrationErr != nil {
			return fmt.Errorf("auto-migration failed for projects: %w", migrationErr)
		}
	} else {
		// Add any missing columns manually
		safeMigrateTable("projects", map[string]string{
			"description": "TEXT",
		})
	}

	if !tableExists("components") {
		migrationErr = DB.AutoMigrate(&Component{})
		if migrationErr != nil {
			return fmt.Errorf("auto-migration failed for components: %w", migrationErr)
		}
	} else {
		safeMigrateTable("components", map[string]string{
			"description": "TEXT",
		})
	}

	if !tableExists("deployments") {
		migrationErr = DB.AutoMigrate(&Deployment{})
		if migrationErr != nil {
			return fmt.Errorf("auto-migration failed for deployments: %w", migrationErr)
		}
	} else {
		// Add new columns that may not exist in older versions
		safeMigrateTable("deployments", map[string]string{
			"change_ticket":     "TEXT",
			"jira_comment_sent": "BOOLEAN DEFAULT false",
			"webhook_sent":      "BOOLEAN DEFAULT false",
		})
	}

	log.Println("✅ Auto-migration completed")

	// Ensure default library exists
	var library Library
	result := DB.First(&library)
	if result.Error == gorm.ErrRecordNotFound {
		library = Library{
			ID:            1,
			Developers:    StringArray{"Kannan"},
			BuildServers:  StringArray{"192.168.1.149"},
			DeployServers: StringArray{},
			Environments:  StringArray{"QA", "UAT", "Production"},
		}
		if err := DB.Create(&library).Error; err != nil {
			log.Printf("⚠️  Warning: Failed to create default library: %v", err)
		} else {
			log.Println("✅ Default library created")
		}
	}

	return nil
}

// CloseDatabase closes the database connection
func CloseDatabase() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// tableExists checks if a table exists in the database
func tableExists(tableName string) bool {
	var count int64
	DB.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
	return count > 0
}

// columnExists checks if a column exists in a table
func columnExists(tableName, columnName string) bool {
	type PragmaInfo struct {
		Name string
	}
	var columns []PragmaInfo
	DB.Raw("PRAGMA table_info(" + tableName + ")").Scan(&columns)
	for _, col := range columns {
		if col.Name == columnName {
			return true
		}
	}
	return false
}

// safeMigrateTable adds missing columns to an existing table without recreating it
func safeMigrateTable(tableName string, columns map[string]string) {
	for colName, colType := range columns {
		if !columnExists(tableName, colName) {
			sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, colName, colType)
			if err := DB.Exec(sql).Error; err != nil {
				log.Printf("⚠️  Warning: Could not add column %s to %s: %v", colName, tableName, err)
			} else {
				log.Printf("✅ Added column %s to %s", colName, tableName)
			}
		}
	}
}
