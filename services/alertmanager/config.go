package alertmanager

import (
	"github.com/pkg/errors"
	"net/url"
	"os"
)

// Configuration options for alertmanager service
type Config struct {
	// Wheather the service should be enabled
	Enabled bool `toml:"enabled" override:"enabled"`
	// URL of the alertmanager endpoint
	URL string `toml:"url" override:"url"`
	// Retry folder
	RetryFolder string `toml:"retry-folder" override:"retry-folder"`
}

func NewConfig() Config {
	return Config{}
}

func (c Config) Validate() error {
	if c.Enabled {
		if c.URL == "" {
			return errors.New("url cannot be empty")
		}
		if _, err := url.Parse(c.URL); err != nil {
			return errors.Wrapf(err, "invalid AlertManager URL: %q", c.URL)
		}
	}
	if _, err := os.Stat(c.RetryFolder); os.IsNotExist(err) {
		return errors.Wrapf(err, "folder %q does not exist", c.RetryFolder)
	}
	return nil
}
