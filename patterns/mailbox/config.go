package mailbox

import (
	"fmt"
	"time"
)

// Config defines the configuration for the Mailbox pattern.
type Config struct {
	Name     string           `yaml:"name" json:"name"`
	Behavior BehaviorConfig   `yaml:"behavior" json:"behavior"`
	Storage  StorageConfig    `yaml:"storage" json:"storage"`
}

// BehaviorConfig defines mailbox behavior settings.
type BehaviorConfig struct {
	Topic          string `yaml:"topic" json:"topic"`
	ConsumerGroup  string `yaml:"consumer_group" json:"consumer_group"`
	AutoCommit     bool   `yaml:"auto_commit" json:"auto_commit"`
}

// StorageConfig defines storage backend settings.
type StorageConfig struct {
	DatabasePath   string        `yaml:"database_path" json:"database_path"`
	TableName      string        `yaml:"table_name" json:"table_name"`
	RetentionDays  int           `yaml:"retention_days" json:"retention_days"`
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("mailbox name is required")
	}

	if c.Behavior.Topic == "" {
		return fmt.Errorf("topic is required")
	}

	if c.Behavior.ConsumerGroup == "" {
		return fmt.Errorf("consumer_group is required")
	}

	if c.Storage.DatabasePath == "" {
		return fmt.Errorf("storage.database_path is required")
	}

	if c.Storage.TableName == "" {
		c.Storage.TableName = "mailbox" // Default table name
	}

	if c.Storage.RetentionDays <= 0 {
		c.Storage.RetentionDays = 90 // Default 90 days retention
	}

	if c.Storage.CleanupInterval <= 0 {
		c.Storage.CleanupInterval = 24 * time.Hour // Default daily cleanup
	}

	return nil
}
