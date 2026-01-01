package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BullionBear/seq/internal/config"
	"github.com/BullionBear/seq/pkg/logger"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("c", "", "Path to configuration file")
	flag.Parse()

	// Determine config path: flag takes precedence over environment variable
	if *configPath == "" {
		*configPath = os.Getenv("CONFIG")
	}

	// Exit if no config path provided
	if *configPath == "" {
		fmt.Fprintf(os.Stderr, "Error: Configuration file path is required.\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -c <config-file> or set CONFIG environment variable\n", os.Args[0])
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load configuration from %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	// Initialize logger (minimal for migrations)
	loggerOpts := logger.Options{
		Level:  "info",
		Output: "stdout",
	}
	if err := logger.Init(loggerOpts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	log := logger.Get()

	// Build database connection string
	dbCfg := cfg.PMS.Database
	sslMode := dbCfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	port := dbCfg.Port
	if port == 0 {
		port = 5432
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		dbCfg.User,
		dbCfg.Password,
		dbCfg.Host,
		port,
		dbCfg.DBName,
		sslMode,
	)

	// Log detailed connection information
	log.Info().
		Str("host", dbCfg.Host).
		Int("port", port).
		Str("database", dbCfg.DBName).
		Str("user", dbCfg.User).
		Str("sslmode", sslMode).
		Str("config_file", *configPath).
		Msg("Database connection configuration")

	// Get migrations directory
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get current working directory")
	}
	migrationsPath := filepath.Join(workDir, "migrations")
	migrationsDir := "file://" + migrationsPath

	log.Info().
		Str("migrations_dir", migrationsPath).
		Msg("Migrations directory")

	// Open database connection
	log.Info().Msg("Opening database connection...")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open database connection")
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal().Err(err).Msg("Failed to ping database")
	}
	log.Info().Msg("Database connection established successfully")

	// Create postgres driver instance
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create postgres driver")
	}

	// Create migrate instance
	m, err := migrate.NewWithDatabaseInstance(migrationsDir, dbCfg.DBName, driver)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create migrate instance")
	}
	defer m.Close()

	// Get current migration version
	currentVersion, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		log.Fatal().Err(err).Msg("Failed to get current migration version")
	}

	if err == migrate.ErrNilVersion {
		log.Info().Msg("No migrations have been applied yet (fresh database)")
	} else {
		log.Info().
			Uint("current_version", currentVersion).
			Bool("dirty", dirty).
			Msg("Current migration state")
		if dirty {
			log.Warn().Msg("Database is in a dirty state - migrations may need manual intervention")
		}
	}

	// List available migration files to show what will be executed
	migrationFiles, err := listMigrationFiles(migrationsPath)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to list migration files (continuing anyway)")
	} else {
		log.Info().
			Int("available_migrations", len(migrationFiles)).
			Msg("Available migration files found")
		for _, mf := range migrationFiles {
			if mf.Direction == "up" {
				// Read and log migration SQL (first few lines for preview)
				preview, err := readMigrationPreview(filepath.Join(migrationsPath, mf.Filename))
				if err == nil {
					log.Info().
						Str("version", mf.Version).
						Str("description", mf.Description).
						Str("sql_preview", preview).
						Msg("Migration file to execute")
				}
			}
		}
	}

	// Run migrations
	log.Info().Msg("Starting migration execution...")
	migrationsApplied := false
	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			log.Info().Msg("No migrations to apply - database is up to date")
			// Continue to verify tables even when no migrations are needed
		} else {
			log.Fatal().Err(err).Msg("Failed to run migrations")
		}
	} else {
		migrationsApplied = true
	}

	// Get version after migration (or current version if no migrations applied)
	finalVersion, dirty, err := m.Version()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get migration version after execution")
	} else {
		if migrationsApplied {
			log.Info().
				Uint("new_version", finalVersion).
				Bool("dirty", dirty).
				Msg("Migration execution completed")
		} else {
			log.Info().
				Uint("current_version", finalVersion).
				Bool("dirty", dirty).
				Msg("Database is at the latest migration version")
		}
	}

	// Verify tables exist by querying the database
	log.Info().Msg("Verifying database schema...")
	tables, err := listTables(db)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to list tables (schema verification skipped)")
	} else {
		log.Info().
			Int("table_count", len(tables)).
			Strs("tables", tables).
			Msg("Database tables verified")
		for _, table := range tables {
			rowCount, err := getTableRowCount(db, table)
			if err == nil {
				log.Info().
					Str("table", table).
					Int64("row_count", rowCount).
					Msg("Table status")
			}
		}
	}

	log.Info().Msg("Migrations completed successfully")
}

// MigrationFile represents a migration file
type MigrationFile struct {
	Version     string
	Description string
	Direction   string
	Filename    string
}

// listMigrationFiles lists all migration files in the migrations directory
func listMigrationFiles(migrationsPath string) ([]MigrationFile, error) {
	var files []MigrationFile
	pattern := regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)

	err := filepath.WalkDir(migrationsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		filename := d.Name()
		matches := pattern.FindStringSubmatch(filename)
		if len(matches) == 4 {
			files = append(files, MigrationFile{
				Version:     matches[1],
				Description: matches[2],
				Direction:   matches[3],
				Filename:    filename,
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by version
	sort.Slice(files, func(i, j int) bool {
		return files[i].Version < files[j].Version
	})

	return files, nil
}

// readMigrationPreview reads the first few lines of a migration file
func readMigrationPreview(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Get first 3 non-empty lines
	var previewLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			previewLines = append(previewLines, trimmed)
			if len(previewLines) >= 3 {
				break
			}
		}
	}

	preview := strings.Join(previewLines, " ")
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}

	return preview, nil
}

// listTables lists all tables in the database
func listTables(db *sql.DB) ([]string, error) {
	query := `
		SELECT tablename 
		FROM pg_tables 
		WHERE schemaname = 'public'
		ORDER BY tablename
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, rows.Err()
}

// getTableRowCount gets the row count for a table
func getTableRowCount(db *sql.DB, tableName string) (int64, error) {
	// Use proper quoting for table name (PostgreSQL uses double quotes)
	query := fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, tableName)
	var count int64
	err := db.QueryRow(query).Scan(&count)
	return count, err
}
