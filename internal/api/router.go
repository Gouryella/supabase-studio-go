package api

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Gouryella/supabase-studio-go/internal/config"
	"github.com/go-chi/chi/v5"
)

type API struct {
	cfg             config.Config
	client          *http.Client
	projectName     string
	projectDiskSize int
	stateFilePath   string
	mu              sync.RWMutex
}

func NewRouter(cfg config.Config) http.Handler {
	api := &API{
		cfg: cfg,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		projectName:     cfg.DefaultProjectName,
		projectDiskSize: cfg.DefaultProjectDiskSizeGB,
		stateFilePath:   cfg.StateFilePath,
	}

	if err := api.ensureManagedFolders(); err != nil {
		log.Printf("failed to create managed folders: %v", err)
	}

	if err := api.loadStateFromDisk(); err != nil {
		log.Printf("failed to load persisted supabase-studio-go state: %v", err)
	}

	r := chi.NewRouter()

	r.Get("/get-ip-address", api.handleGetIPAddress)
	r.Get("/get-utc-time", api.handleGetUTCTime)
	r.Get("/get-deployment-commit", api.handleDeploymentCommit)
	r.Get("/cli-release-version", api.handleCLIReleaseVersion)
	r.Get("/check-cname", api.handleCheckCNAME)
	r.Post("/generate-attachment-url", api.handleGenerateAttachmentURL)
	r.Post("/edge-functions/test", api.handleEdgeFunctionTest)
	r.Get("/incident-status", api.handleIncidentStatus)
	r.MethodNotAllowed(api.methodNotAllowed)

	r.Post("/mcp", api.handleMCP)
	r.Route("/ai", func(r chi.Router) {
		r.Get("/sql/check-api-key", api.handleCheckAPIKey)
		r.Post("/sql/generate-v4", api.handleAISQLGenerateV4)
		r.Post("/sql/policy", api.handleAISQLPolicy)
		r.Post("/sql/cron-v2", api.handleAISQLCronV2)
		r.Post("/sql/title-v2", api.handleAISQLTitleV2)
		r.Post("/sql/filter-v1", api.handleAISQLFilterV1)
		r.Post("/code/complete", api.handleAICodeComplete)
		r.Post("/feedback/rate", api.handleAIFeedbackRate)
		r.Post("/feedback/classify", api.handleAIFeedbackClassify)
		r.Post("/docs", api.handleAIDocs)
		r.Post("/onboarding/design", api.handleAIOnboardingDesign)
	})
	r.Route("/integrations", func(r chi.Router) {
		r.MethodFunc("POST", "/stripe-sync", api.handleStripeSync)
		r.MethodFunc("DELETE", "/stripe-sync", api.handleStripeSync)
	})
	r.Get("/connect", api.handleConnectContent)

	r.Route("/platform", func(r chi.Router) {
		r.Route("/pg-meta/{ref}", func(r chi.Router) {
			r.Get("/tables", api.pgMetaProxy("tables"))
			r.Get("/views", api.pgMetaProxy("views"))
			r.Get("/policies", api.pgMetaProxy("policies"))
			r.Get("/column-privileges", api.pgMetaProxy("column-privileges"))
			r.Get("/foreign-tables", api.pgMetaProxy("foreign-tables"))
			r.Get("/extensions", api.pgMetaProxy("extensions"))
			r.Get("/types", api.pgMetaProxy("types"))
			r.Get("/materialized-views", api.pgMetaProxy("materialized-views"))
			r.Get("/publications", api.pgMetaProxy("publications"))
			r.Get("/triggers", api.pgMetaProxy("triggers"))
			r.Post("/query", api.handlePgMetaQuery)
		})

		r.Route("/storage/{ref}", func(r chi.Router) {
			r.Route("/buckets", func(r chi.Router) {
				r.Get("/", api.handleStorageBuckets)
				r.Post("/", api.handleStorageBuckets)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", api.handleStorageBucket)
					r.Patch("/", api.handleStorageBucket)
					r.Delete("/", api.handleStorageBucket)
					r.Post("/empty", api.handleStorageEmptyBucket)
					r.Route("/objects", func(r chi.Router) {
						r.Delete("/", api.handleStorageObjectsDelete)
						r.Post("/list", api.handleStorageObjectsList)
						r.Post("/public-url", api.handleStorageObjectsPublicURL)
						r.Post("/download", api.handleStorageObjectsDownload)
						r.Post("/move", api.handleStorageObjectsMove)
						r.Post("/sign", api.handleStorageObjectsSign)
					})
				})
			})
		})

		r.Route("/auth/{ref}", func(r chi.Router) {
			r.Post("/invite", api.handleAuthInvite)
			r.Post("/magiclink", api.handleAuthMagicLink)
			r.Post("/recover", api.handleAuthRecover)
			r.Post("/otp", api.handleAuthOTP)
			r.Route("/users", func(r chi.Router) {
				r.Post("/", api.handleAuthUsersCreate)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", api.handleAuthUser)
					r.Put("/", api.handleAuthUser)
					r.Delete("/", api.handleAuthUser)
					r.Delete("/factors", api.handleAuthUserFactors)
				})
			})
		})

		r.Route("/projects", func(r chi.Router) {
			r.Get("/", api.handleProjectsList)
			r.Route("/{ref}", func(r chi.Router) {
				r.Get("/", api.handleProjectDetail)
				r.Patch("/", api.handleProjectUpdate)
				r.Get("/settings", api.handleProjectSettings)
				r.Get("/databases", api.handleProjectDatabases)
				r.Get("/disk", api.handleProjectDisk)
				r.Post("/disk", api.handleProjectDisk)
				r.Get("/disk/util", api.handleProjectDiskUtilization)
				r.Post("/resize", api.handleProjectResize)
				r.Get("/api/rest", api.handleProjectRest)
				r.Head("/api/rest", api.handleProjectRest)
				r.Get("/api/graphql", api.handleProjectGraphql)
				r.Post("/api-keys/temporary", api.handleProjectTempAPIKey)
				r.Get("/infra-monitoring", api.handleProjectInfraMonitoring)
				r.Get("/billing/addons", api.handleProjectBillingAddons)
				r.Route("/config", func(r chi.Router) {
					r.Get("/", api.handleProjectConfig)
					r.Patch("/", api.handleProjectConfig)
					r.Get("/postgrest", api.handleProjectPostgrestConfig)
				})
				r.Route("/analytics", func(r chi.Router) {
					r.Get("/log-drains", api.handleProjectLogDrains)
					r.Post("/log-drains", api.handleProjectLogDrains)
					r.Route("/log-drains/{uuid}", func(r chi.Router) {
						r.Get("/", api.handleProjectLogDrain)
						r.Put("/", api.handleProjectLogDrain)
						r.Delete("/", api.handleProjectLogDrain)
					})
					r.Route("/endpoints/{name}", func(r chi.Router) {
						r.Get("/", api.handleProjectAnalyticsEndpoint)
						r.Post("/", api.handleProjectAnalyticsEndpoint)
					})
				})
				r.Route("/content", func(r chi.Router) {
					r.Get("/", api.handleSnippets)
					r.Put("/", api.handleSnippets)
					r.Delete("/", api.handleSnippets)
					r.Get("/count", api.handleSnippetCount)
					r.Route("/folders", func(r chi.Router) {
						r.Get("/", api.handleSnippetFolders)
						r.Post("/", api.handleSnippetFolders)
						r.Route("/{id}", func(r chi.Router) {
							r.Get("/", api.handleSnippetFolderByID)
							r.Delete("/", api.handleSnippetFolderByID)
						})
					})
					r.Route("/item/{id}", func(r chi.Router) {
						r.Get("/", api.handleSnippetItem)
						r.Put("/", api.handleSnippetItem)
						r.Delete("/", api.handleSnippetItem)
					})
				})
				r.Get("/run-lints", api.handleRunLints)
			})
		})

		r.Get("/organizations", api.handleOrganizations)
		r.Get("/organizations/{slug}/billing/subscription", api.handleOrgSubscription)
		r.Route("/database/{ref}", func(r chi.Router) {
			r.Get("/pooling", api.handleDatabasePooling)
			r.Patch("/pooling", api.handleDatabasePooling)
		})

		r.Route("/props", func(r chi.Router) {
			r.Get("/project/{ref}", api.handlePropsProject)
			r.Get("/project/{ref}/api", api.handlePropsProjectAPI)
			r.Get("/org/{slug}", api.handlePropsOrg)
		})

		r.Route("/integrations", func(r chi.Router) {
			r.Get("/github/connections", api.handleGithubConnections)
			r.Get("/github/authorization", api.handleGithubAuthorization)
			r.Get("/github/repositories", api.handleGithubRepositories)
			r.Get("/{slug}", api.handleIntegrationBySlug)
		})

		r.Get("/profile", api.handleProfile)
		r.Post("/telemetry/event", api.handleTelemetryEvent)
	})

	r.Route("/v1/projects/{ref}", func(r chi.Router) {
		r.Get("/api-keys", api.handleV1ApiKeys)
		r.Route("/functions", func(r chi.Router) {
			r.Get("/", api.handleFunctions)
			r.Get("/{slug}", api.handleFunctionBySlug)
		})
		r.Get("/types/typescript", api.handleTypescriptTypes)
		r.Route("/database/migrations", func(r chi.Router) {
			r.Get("/", api.handleMigrations)
			r.Post("/", api.handleMigrations)
		})
	})

	return r
}
