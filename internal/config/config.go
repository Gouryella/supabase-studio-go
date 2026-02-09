package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddress string
	BasePath      string
	IsPlatform    bool
	StateFilePath string

	SupabaseURL        string
	SupabasePublicURL  string
	SupabaseAnonKey    string
	SupabaseServiceKey string

	StudioPgMetaURL string
	PgMetaCryptoKey string

	PostgresHost          string
	PostgresPort          string
	PostgresDatabase      string
	PostgresPassword      string
	PostgresUserReadWrite string
	PostgresUserReadOnly  string

	LogflareURL   string
	LogflareToken string

	SupportAPIURL string
	SupportAPIKey string

	EdgeFunctionsFolder string
	SnippetsFolder      string

	CustomerDomain string
	APIDomain      string

	DefaultOrganizationName  string
	DefaultProjectName       string
	DefaultProjectDiskSizeGB int

	AuthJWTSecret string
}

func Load() Config {
	return Config{
		ListenAddress: envFirst("SUPABASE_STUDIO_GO_LISTEN", "STUDIO_GO_LISTEN"),
		BasePath:      os.Getenv("NEXT_PUBLIC_BASE_PATH"),
		IsPlatform:    strings.EqualFold(os.Getenv("NEXT_PUBLIC_IS_PLATFORM"), "true"),
		StateFilePath: envOrAny(defaultStateFilePath(), "SUPABASE_STUDIO_GO_STATE_FILE", "STUDIO_GO_STATE_FILE"),

		SupabaseURL:       os.Getenv("SUPABASE_URL"),
		SupabasePublicURL: os.Getenv("SUPABASE_PUBLIC_URL"),
		SupabaseAnonKey:   os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseServiceKey: envFirst(
			"SUPABASE_SERVICE_KEY",
			"SUPABASE_SERVICE_ROLE_KEY",
			"SERVICE_ROLE_KEY",
			"SERVICE_KEY",
		),

		StudioPgMetaURL: os.Getenv("STUDIO_PG_META_URL"),
		PgMetaCryptoKey: envOr("PG_META_CRYPTO_KEY", "SAMPLE_KEY"),

		PostgresHost:          envOr("POSTGRES_HOST", "db"),
		PostgresPort:          envOr("POSTGRES_PORT", "5432"),
		PostgresDatabase:      envOr("POSTGRES_DB", "postgres"),
		PostgresPassword:      envOr("POSTGRES_PASSWORD", "postgres"),
		PostgresUserReadWrite: envOr("POSTGRES_USER_READ_WRITE", "supabase_admin"),
		PostgresUserReadOnly:  envOr("POSTGRES_USER_READ_ONLY", "supabase_read_only_user"),

		LogflareURL:   os.Getenv("LOGFLARE_URL"),
		LogflareToken: os.Getenv("LOGFLARE_PRIVATE_ACCESS_TOKEN"),

		SupportAPIURL: os.Getenv("NEXT_PUBLIC_SUPPORT_API_URL"),
		SupportAPIKey: os.Getenv("SUPPORT_SUPABASE_SECRET_KEY"),

		EdgeFunctionsFolder: os.Getenv("EDGE_FUNCTIONS_MANAGEMENT_FOLDER"),
		SnippetsFolder:      os.Getenv("SNIPPETS_MANAGEMENT_FOLDER"),

		CustomerDomain: os.Getenv("NEXT_PUBLIC_CUSTOMER_DOMAIN"),
		APIDomain:      os.Getenv("NEXT_PUBLIC_API_DOMAIN"),

		DefaultOrganizationName:  envOr("DEFAULT_ORGANIZATION_NAME", "Default Organization"),
		DefaultProjectName:       envOr("DEFAULT_PROJECT_NAME", "Default Project"),
		DefaultProjectDiskSizeGB: envOrInt("DEFAULT_PROJECT_DISK_SIZE_GB", 8),

		AuthJWTSecret: envOr("AUTH_JWT_SECRET", "super-secret-jwt-token-with-at-least-32-characters-long"),
	}
}

func defaultStateFilePath() string {
	path := filepath.Join(os.TempDir(), "supabase-studio-go", "state.json")
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		path = filepath.Join(dir, "supabase-studio-go", "state.json")
	}

	migrateLegacyStateFile(path)
	return path
}

func migrateLegacyStateFile(targetPath string) {
	if strings.TrimSpace(targetPath) == "" {
		return
	}

	if _, err := os.Stat(targetPath); err == nil {
		return
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}

	legacyPath := filepath.Join(".supabase-studio-go", "state.json")
	bytes, err := os.ReadFile(legacyPath)
	if err != nil {
		return
	}

	dir := filepath.Dir(targetPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}

	_ = os.WriteFile(targetPath, bytes, 0o644)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func envOrAny(fallback string, keys ...string) string {
	if value := envFirst(keys...); value != "" {
		return value
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
