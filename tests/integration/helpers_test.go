//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pci-vault/vault/internal/audit"
	"github.com/pci-vault/vault/internal/auth"
	"github.com/pci-vault/vault/internal/handler"
	"github.com/pci-vault/vault/internal/model"
	"github.com/pci-vault/vault/internal/kms"
	"github.com/pci-vault/vault/internal/proxy"
	vaultredis "github.com/pci-vault/vault/internal/redis"
	"github.com/pci-vault/vault/internal/repository"
	"github.com/pci-vault/vault/internal/server"
)

type testEnv struct {
	tokenizerURL string
	proxyURL     string
	pool         *pgxpool.Pool
	rdb          *goredis.Client
	kmsKeyARN    string
	hmacKey      []byte
	cleanup      func()
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()

	// --- Containers ---
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("vault_test"),
		tcpostgres.WithUsername("vault"),
		tcpostgres.WithPassword("vault"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}

	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("pg conn: %v", err)
	}

	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}
	redisEndpoint, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("redis endpoint: %v", err)
	}

	lsContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "localstack/localstack",
			ExposedPorts: []string{"4566/tcp"},
			Env:          map[string]string{"SERVICES": "kms", "DEBUG": "0"},
			WaitingFor:   wait.ForHTTP("/_localstack/health").WithPort("4566/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start localstack: %v", err)
	}
	lsHost, _ := lsContainer.Host(ctx)
	lsPort, _ := lsContainer.MappedPort(ctx, "4566/tcp")
	lsEndpoint := fmt.Sprintf("http://%s:%s", lsHost, lsPort.Port())

	// --- Migrations ---
	migrationsPath := findMigrationsDir(t)
	m, err := migrate.New("file://"+migrationsPath, pgConnStr)
	if err != nil {
		t.Fatalf("init migrate: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}
	m.Close()

	// --- Clients ---
	pool, err := pgxpool.New(ctx, pgConnStr)
	if err != nil {
		t.Fatalf("init pool: %v", err)
	}

	rdb := goredis.NewClient(&goredis.Options{Addr: redisEndpoint})

	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	keyARN := createKMSKey(t, ctx, lsEndpoint)

	kmsClient, err := kms.New(ctx, keyARN, "us-east-1", lsEndpoint)
	if err != nil {
		t.Fatalf("init kms: %v", err)
	}
	hmacKey := []byte("test-hmac-key-for-integration-00")

	// --- Repos & Services ---
	tokenRepo := repository.NewTokenRepo(pool)
	vaultRepo := repository.NewVaultRepo(pool)
	auditRepo := repository.NewAuditRepo(pool)
	tenantRepo := repository.NewTenantRepo(pool)
	cvvStore := vaultredis.NewCVVStore(rdb)
	auditLogger := audit.NewLogger(auditRepo)

	// Create a test tenant
	testTenant := &model.Tenant{
		TenantID:  "test-tenant",
		Name:      "Test Tenant",
		Status:    model.TenantStatusActive,
		KMSKeyARN: keyARN,
	}
	if err := tenantRepo.Create(ctx, testTenant); err != nil {
		t.Fatalf("create test tenant: %v", err)
	}

	// --- Tokenizer Server ---
	tokenizeH := handler.NewTokenizeHandler(
		tokenRepo, vaultRepo, cvvStore, kmsClient, auditLogger,
		hmacKey, 1*time.Hour, keyARN,
	)
	detokenizeH := handler.NewDetokenizeHandler(
		tokenRepo, vaultRepo, cvvStore, kmsClient, auditLogger,
	)
	tokenManageH := handler.NewTokenManageHandler(tokenRepo, auditRepo, cvvStore, auditLogger)
	healthH := handler.NewHealthHandler(pool, rdb, func(ctx context.Context) bool { return true })

	tenantH := handler.NewTenantHandler(tenantRepo, kmsClient)
	tenantMiddleware := auth.RequireTenant(tenantRepo)

	tr := server.NewRouter()
	tr.Get("/health", healthH.ServeHTTP)

	// Admin routes (no tenant middleware)
	tr.Group(func(r chi.Router) {
		r.Use(auth.BearerAuth)
		r.Post("/admin/tenants", tenantH.Create)
		r.Get("/admin/tenants", tenantH.List)
		r.Get("/admin/tenants/{tenant_id}", tenantH.Get)
		r.Delete("/admin/tenants/{tenant_id}", tenantH.Deactivate)
	})
	tr.Group(func(r chi.Router) {
		r.Use(auth.BearerAuth)
		r.Use(tenantMiddleware)
		r.Post("/vault/tokenize", tokenizeH.ServeHTTP)
		r.Get("/vault/tokens/{token_id}", tokenManageH.GetStatus)
		r.Delete("/vault/tokens/{token_id}", tokenManageH.Deactivate)
		r.Get("/vault/tokens/{token_id}/audit", tokenManageH.GetAuditTrail)
		r.With(auth.RequireMTLSCert("proxy")).
			Post("/internal/detokenize", detokenizeH.ServeHTTP)
	})
	tokenizerSrv := httptest.NewServer(tr)

	// --- Proxy Server ---
	tokenizerHTTPClient := &http.Client{Timeout: 10 * time.Second}
	revealer := proxy.NewRevealer(tokenizerHTTPClient, tokenizerSrv.URL, "Bearer proxy:secret")
	forwarder := proxy.NewForwarder(&http.Client{Timeout: 10 * time.Second})
	forwardH := handler.NewForwardHandler(revealer, forwarder)
	proxyHealthH := handler.NewProxyHealthHandler()

	pr := server.NewRouter()
	pr.Get("/health", proxyHealthH.ServeHTTP)
	pr.Group(func(r chi.Router) {
		r.Use(auth.BearerAuth)
		r.Use(auth.ExtractTenantHeader)
		r.Post("/proxy/forward", forwardH.ServeHTTP)
	})
	proxySrv := httptest.NewServer(pr)

	cleanup := func() {
		tokenizerSrv.Close()
		proxySrv.Close()
		pool.Close()
		rdb.Close()
		pgContainer.Terminate(ctx)
		redisContainer.Terminate(ctx)
		lsContainer.Terminate(ctx)
	}

	return &testEnv{
		tokenizerURL: tokenizerSrv.URL,
		proxyURL:     proxySrv.URL,
		pool:         pool,
		rdb:          rdb,
		kmsKeyARN:    keyARN,
		hmacKey:      hmacKey,
		cleanup:      cleanup,
	}
}

func createKMSKey(t *testing.T, ctx context.Context, endpoint string) string {
	t.Helper()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"))
	if err != nil {
		t.Fatalf("aws config: %v", err)
	}
	client := awskms.NewFromConfig(cfg, func(o *awskms.Options) {
		o.BaseEndpoint = &endpoint
	})

	out, err := client.CreateKey(ctx, &awskms.CreateKeyInput{
		Description: strPtr("test vault key"),
	})
	if err != nil {
		t.Fatalf("create kms key: %v", err)
	}
	return *out.KeyMetadata.Arn
}

func strPtr(s string) *string { return &s }

func findMigrationsDir(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("migrations directory not found")
	return ""
}

// --- HTTP helpers ---

const defaultTestTenant = "test-tenant"

func doPost(t *testing.T, url string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client:secret")
	req.Header.Set("X-Tenant-ID", defaultTestTenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp, readJSON(t, resp)
}

func doPostAs(t *testing.T, url, identity string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+identity+":secret")
	req.Header.Set("X-Tenant-ID", defaultTestTenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp, readJSON(t, resp)
}

func doGet(t *testing.T, url string) (*http.Response, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer client:secret")
	req.Header.Set("X-Tenant-ID", defaultTestTenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp, readJSON(t, resp)
}

func doDelete(t *testing.T, url string) (*http.Response, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer client:secret")
	req.Header.Set("X-Tenant-ID", defaultTestTenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	return resp, readJSON(t, resp)
}

func doRaw(t *testing.T, method, url string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func readJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(b, &result)
	return result
}
