package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	version = "0.1.0"
)

var (
	// CLI flags
	pluginsDir    string
	debounceMs    int
	autoBuild     bool
	autoReload    bool
	buildTarget   string
	podmanBinary  string
	verboseOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "prism-watcher",
	Short: "File watcher for Prism backend plugins",
	Long: `prism-watcher monitors the plugins directory and automatically
rebuilds container images when source files change.

Based on ADR-025 (Container Plugin Model), this enables rapid
development iteration with automatic rebuilds and optional
container restarts.`,
	Version: version,
	RunE:    runWatcher,
}

func init() {
	rootCmd.Flags().StringVar(&pluginsDir, "dir", "./plugins", "Plugins directory to watch")
	rootCmd.Flags().IntVar(&debounceMs, "debounce", 1000, "Debounce delay in milliseconds")
	rootCmd.Flags().BoolVar(&autoBuild, "build", true, "Automatically build on file changes")
	rootCmd.Flags().BoolVar(&autoReload, "reload", false, "Automatically restart containers after build")
	rootCmd.Flags().StringVar(&buildTarget, "target", "production", "Build target (production or debug)")
	rootCmd.Flags().StringVar(&podmanBinary, "podman", "podman", "Path to podman binary")
	rootCmd.Flags().BoolVarP(&verboseOutput, "verbose", "v", false, "Verbose output")

	viper.BindPFlag("dir", rootCmd.Flags().Lookup("dir"))
	viper.BindPFlag("debounce", rootCmd.Flags().Lookup("debounce"))
	viper.BindPFlag("build", rootCmd.Flags().Lookup("build"))
	viper.BindPFlag("reload", rootCmd.Flags().Lookup("reload"))
	viper.BindPFlag("target", rootCmd.Flags().Lookup("target"))
	viper.BindPFlag("podman", rootCmd.Flags().Lookup("podman"))
}

func main() {
	// Setup structured logging
	logLevel := slog.LevelInfo
	if verboseOutput {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func runWatcher(cmd *cobra.Command, args []string) error {
	slog.Info("starting prism plugin watcher",
		"version", version,
		"dir", pluginsDir,
		"debounce_ms", debounceMs,
		"auto_build", autoBuild,
		"auto_reload", autoReload,
		"target", buildTarget)

	// Verify plugins directory exists
	absDir, err := filepath.Abs(pluginsDir)
	if err != nil {
		return fmt.Errorf("failed to resolve plugins directory: %w", err)
	}

	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		return fmt.Errorf("plugins directory does not exist: %s", absDir)
	}

	// Verify podman is available
	if _, err := exec.LookPath(podmanBinary); err != nil {
		return fmt.Errorf("podman not found (use --podman to specify path): %w", err)
	}

	// Create file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Close()

	// Add plugin directories to watch
	plugins := []string{"core", "postgres", "kafka", "redis"}
	for _, plugin := range plugins {
		pluginDir := filepath.Join(absDir, plugin)
		if err := addWatchRecursive(watcher, pluginDir); err != nil {
			slog.Warn("failed to watch plugin directory", "plugin", plugin, "error", err)
		} else {
			slog.Info("watching plugin", "plugin", plugin, "path", pluginDir)
		}
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("received shutdown signal")
		cancel()
	}()

	// Start watching
	return watchLoop(ctx, watcher, absDir)
}

// addWatchRecursive adds a directory and all its subdirectories to the watcher
func addWatchRecursive(watcher *fsnotify.Watcher, path string) error {
	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only watch directories
		if info.IsDir() {
			// Skip hidden directories and vendor/target dirs
			if strings.HasPrefix(info.Name(), ".") ||
			   info.Name() == "vendor" ||
			   info.Name() == "target" {
				return filepath.SkipDir
			}

			if err := watcher.Add(walkPath); err != nil {
				return fmt.Errorf("failed to watch %s: %w", walkPath, err)
			}
		}

		return nil
	})
}

// watchLoop is the main event loop
func watchLoop(ctx context.Context, watcher *fsnotify.Watcher, pluginsDir string) error {
	// Debounce timer
	var debounceTimer *time.Timer
	pendingChanges := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping watcher")
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("watcher events channel closed")
			}

			// Only care about write and create events for Go files
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			// Only watch Go source files
			if !strings.HasSuffix(event.Name, ".go") &&
			   !strings.HasSuffix(event.Name, ".mod") {
				continue
			}

			plugin := detectPlugin(event.Name, pluginsDir)
			if plugin == "" {
				continue
			}

			slog.Debug("file changed", "file", filepath.Base(event.Name), "plugin", plugin, "op", event.Op)
			pendingChanges[plugin] = true

			// Reset debounce timer
			if debounceTimer != nil {
				debounceTimer.Stop()
			}

			debounceTimer = time.AfterFunc(time.Duration(debounceMs)*time.Millisecond, func() {
				for changedPlugin := range pendingChanges {
					slog.Info("detected changes", "plugin", changedPlugin)

					if autoBuild {
						if err := buildPlugin(changedPlugin, pluginsDir); err != nil {
							slog.Error("build failed", "plugin", changedPlugin, "error", err)
						} else {
							slog.Info("build successful", "plugin", changedPlugin)

							if autoReload {
								if err := reloadPlugin(changedPlugin); err != nil {
									slog.Error("reload failed", "plugin", changedPlugin, "error", err)
								} else {
									slog.Info("reload successful", "plugin", changedPlugin)
								}
							}
						}
					}
				}

				// Clear pending changes
				pendingChanges = make(map[string]bool)
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("watcher errors channel closed")
			}
			slog.Error("watcher error", "error", err)
		}
	}
}

// detectPlugin determines which plugin a file belongs to
func detectPlugin(filePath, pluginsDir string) string {
	rel, err := filepath.Rel(pluginsDir, filePath)
	if err != nil {
		return ""
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}

	plugin := parts[0]
	// Only return known plugins
	if plugin == "core" || plugin == "postgres" || plugin == "kafka" || plugin == "redis" {
		return plugin
	}

	return ""
}

// buildPlugin builds a plugin container image
func buildPlugin(plugin, pluginsDir string) error {
	// Core is built as part of other plugins, skip standalone build
	if plugin == "core" {
		slog.Info("core package changed, rebuilding all plugins")
		// Rebuild all plugins that depend on core
		for _, p := range []string{"postgres", "kafka", "redis"} {
			if err := buildPlugin(p, pluginsDir); err != nil {
				return err
			}
		}
		return nil
	}

	pluginDir := filepath.Join(pluginsDir, plugin)
	dockerfilePath := filepath.Join(pluginDir, "Dockerfile")

	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return fmt.Errorf("Dockerfile not found: %s", dockerfilePath)
	}

	registry := "prism"
	imageName := fmt.Sprintf("%s/%s-plugin:%s", registry, plugin, buildTarget)
	latestTag := fmt.Sprintf("%s/%s-plugin:latest", registry, plugin)

	slog.Info("building plugin",
		"plugin", plugin,
		"target", buildTarget,
		"image", imageName)

	args := []string{
		"build",
		"--target", buildTarget,
		"-t", imageName,
	}

	// Also tag as latest for production builds
	if buildTarget == "production" {
		args = append(args, "-t", latestTag)
	}

	args = append(args, "-f", dockerfilePath, pluginsDir)

	cmd := exec.Command(podmanBinary, args...)
	cmd.Dir = pluginDir

	if verboseOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	start := time.Now()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	slog.Info("build completed",
		"plugin", plugin,
		"duration", time.Since(start).Round(time.Millisecond))

	return nil
}

// reloadPlugin stops and restarts a running plugin container
func reloadPlugin(plugin string) error {
	containerName := fmt.Sprintf("%s-plugin", plugin)
	registry := "prism"
	imageName := fmt.Sprintf("%s/%s-plugin:latest", registry, plugin)

	// Stop existing container (ignore errors if not running)
	stopCmd := exec.Command(podmanBinary, "stop", containerName)
	stopCmd.Run() // Ignore error

	// Remove existing container (ignore errors if doesn't exist)
	rmCmd := exec.Command(podmanBinary, "rm", containerName)
	rmCmd.Run() // Ignore error

	// Start new container
	slog.Info("starting plugin container", "plugin", plugin)

	args := []string{
		"run", "-d",
		"--name", containerName,
		"-p", getPluginPort(plugin),
	}

	// Add environment variables based on plugin
	switch plugin {
	case "postgres":
		args = append(args,
			"-e", "DATABASE_URL=postgres://prism:password@host.containers.internal:5432/prism",
			"-e", "PRISM_LOG_LEVEL=debug")
	case "kafka":
		args = append(args,
			"-e", "KAFKA_BROKERS=host.containers.internal:9092",
			"-e", "KAFKA_TOPIC=events",
			"-e", "PRISM_LOG_LEVEL=debug")
	case "redis":
		args = append(args,
			"-e", "REDIS_ADDRESS=host.containers.internal:6379",
			"-e", "PRISM_LOG_LEVEL=debug")
	}

	args = append(args, imageName)

	cmd := exec.Command(podmanBinary, args...)
	if verboseOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	slog.Info("plugin container started", "plugin", plugin, "container", containerName)
	return nil
}

// getPluginPort returns the control plane port for a plugin
func getPluginPort(plugin string) string {
	switch plugin {
	case "postgres":
		return "9090:9090"
	case "kafka":
		return "9091:9091"
	case "redis":
		return "9092:9092"
	default:
		return "9090:9090"
	}
}
