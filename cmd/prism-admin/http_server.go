package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jrepp/prism-data-layer/cmd/prism-admin/templates"
)

// HTTPServer wraps the Gin HTTP server for the admin UI
type HTTPServer struct {
	storage *Storage
	router  *gin.Engine
	server  *http.Server
}

// NewHTTPServer creates a new HTTP server for the admin UI
func NewHTTPServer(storage *Storage, port int) *HTTPServer {
	gin.SetMode(gin.ReleaseMode) // Use release mode for cleaner output

	router := gin.Default()

	// Serve static files
	router.Static("/static", "./cmd/prism-admin/static")

	hs := &HTTPServer{
		storage: storage,
		router:  router,
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: router,
		},
	}

	// Register routes
	hs.registerRoutes()

	return hs
}

// registerRoutes sets up all HTTP routes
func (hs *HTTPServer) registerRoutes() {
	// Dashboard
	hs.router.GET("/", hs.handleDashboard)
	hs.router.GET("/api/metrics", hs.handleMetrics)

	// Proxies
	hs.router.GET("/proxies", hs.handleProxies)

	// Launchers
	hs.router.GET("/launchers", hs.handleLaunchers)

	// Namespaces
	hs.router.GET("/namespaces", hs.handleNamespaces)
}

// handleDashboard renders the dashboard page
func (hs *HTTPServer) handleDashboard(c *gin.Context) {
	ctx := c.Request.Context()
	metrics := hs.getDashboardMetrics(ctx)

	c.Header("Content-Type", "text/html")
	templates.DashboardPage(metrics).Render(ctx, c.Writer)
}

// handleMetrics returns just the metrics grid for htmx polling
func (hs *HTTPServer) handleMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	metrics := hs.getDashboardMetrics(ctx)

	c.Header("Content-Type", "text/html")
	templates.MetricsGrid(metrics).Render(ctx, c.Writer)
}

// getDashboardMetrics queries the storage for dashboard metrics
func (hs *HTTPServer) getDashboardMetrics(ctx context.Context) *templates.DashboardMetrics {
	proxies, _ := hs.storage.ListProxies(ctx)
	launchers, _ := hs.storage.ListLaunchers(ctx)
	namespaces, _ := hs.storage.ListNamespaces(ctx)

	proxyHealthy := 0
	proxyUnhealthy := 0
	for _, p := range proxies {
		if p.Status == "healthy" {
			proxyHealthy++
		} else {
			proxyUnhealthy++
		}
	}

	launcherHealthy := 0
	launcherUnhealthy := 0
	for _, l := range launchers {
		if l.Status == "healthy" {
			launcherHealthy++
		} else {
			launcherUnhealthy++
		}
	}

	return &templates.DashboardMetrics{
		ProxyCount:        len(proxies),
		ProxyHealthy:      proxyHealthy,
		ProxyUnhealthy:    proxyUnhealthy,
		LauncherCount:     len(launchers),
		LauncherHealthy:   launcherHealthy,
		LauncherUnhealthy: launcherUnhealthy,
		NamespaceCount:    len(namespaces),
		LastUpdate:        time.Now().Format("15:04:05"),
	}
}

// handleProxies renders the proxies page
func (hs *HTTPServer) handleProxies(c *gin.Context) {
	ctx := c.Request.Context()
	proxies, err := hs.storage.ListProxies(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading proxies: %v", err)
		return
	}

	proxyInfos := make([]*templates.ProxyInfo, len(proxies))
	for i, p := range proxies {
		proxyInfos[i] = &templates.ProxyInfo{
			ProxyID:  p.ProxyID,
			Address:  p.Address,
			Version:  p.Version,
			Status:   p.Status,
			LastSeen: p.LastSeen,
		}
	}

	c.Header("Content-Type", "text/html")
	templates.ProxiesPage(proxyInfos).Render(ctx, c.Writer)
}

// handleLaunchers renders the launchers page
func (hs *HTTPServer) handleLaunchers(c *gin.Context) {
	ctx := c.Request.Context()
	launchers, err := hs.storage.ListLaunchers(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading launchers: %v", err)
		return
	}

	launcherInfos := make([]*templates.LauncherInfo, len(launchers))
	for i, l := range launchers {
		launcherInfos[i] = &templates.LauncherInfo{
			LauncherID:     l.LauncherID,
			Address:        l.Address,
			Region:         l.Region,
			Version:        l.Version,
			Status:         l.Status,
			MaxProcesses:   l.MaxProcesses,
			AvailableSlots: l.AvailableSlots,
			LastSeen:       l.LastSeen,
		}
	}

	c.Header("Content-Type", "text/html")
	templates.LaunchersPage(launcherInfos).Render(ctx, c.Writer)
}

// handleNamespaces renders the namespaces page
func (hs *HTTPServer) handleNamespaces(c *gin.Context) {
	ctx := c.Request.Context()
	namespaces, err := hs.storage.ListNamespaces(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading namespaces: %v", err)
		return
	}

	namespaceInfos := make([]*templates.NamespaceInfo, len(namespaces))
	for i, ns := range namespaces {
		namespaceInfos[i] = &templates.NamespaceInfo{
			Name:        ns.Name,
			Description: ns.Description,
			CreatedAt:   ns.CreatedAt,
			UpdatedAt:   ns.UpdatedAt,
		}
	}

	c.Header("Content-Type", "text/html")
	templates.NamespacesPage(namespaceInfos).Render(ctx, c.Writer)
}

// Start starts the HTTP server
func (hs *HTTPServer) Start() error {
	fmt.Printf("[INFO] HTTP server starting on %s\n", hs.server.Addr)
	return hs.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server
func (hs *HTTPServer) Shutdown(ctx context.Context) error {
	fmt.Printf("[INFO] HTTP server shutting down...\n")
	return hs.server.Shutdown(ctx)
}
