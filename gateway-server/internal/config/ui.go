package config

import (
	"fmt"
	"net"
	"path"
	"strings"
)

type UIConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Mode       string `yaml:"mode"`
	ListenIP   string `yaml:"listen_ip"`
	ListenPort int    `yaml:"listen_port"`
	BasePath   string `yaml:"base_path"`
}

func DefaultUIConfig() UIConfig {
	return UIConfig{
		Enabled:    true,
		Mode:       "external",
		ListenIP:   "127.0.0.1",
		ListenPort: 18080,
		BasePath:   "/",
	}
}

func (c *UIConfig) ApplyDefaults(defaultPort int) {
	d := DefaultUIConfig()
	if strings.TrimSpace(c.Mode) == "" {
		c.Mode = d.Mode
	} else {
		c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	}
	if strings.TrimSpace(c.ListenIP) == "" {
		c.ListenIP = d.ListenIP
	}
	if c.ListenPort == 0 {
		if defaultPort > 0 {
			c.ListenPort = defaultPort
		} else {
			c.ListenPort = d.ListenPort
		}
	}
	if strings.TrimSpace(c.BasePath) == "" {
		c.BasePath = d.BasePath
	}
	c.BasePath = normalizeBasePath(c.BasePath)
}

func (c UIConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	var errs []error
	if c.Mode != "external" && c.Mode != "embedded" {
		errs = append(errs, fmt.Errorf("ui.mode %q is unsupported", c.Mode))
	}
	if net.ParseIP(c.ListenIP) == nil {
		errs = append(errs, fmt.Errorf("ui.listen_ip %q is invalid", c.ListenIP))
	}
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		errs = append(errs, fmt.Errorf("ui.listen_port %d out of range [1,65535]", c.ListenPort))
	}
	if !strings.HasPrefix(c.BasePath, "/") {
		errs = append(errs, fmt.Errorf("ui.base_path %q must start with /", c.BasePath))
	}
	return joinErrors(errs)
}

func normalizeBasePath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	if p == "." {
		return "/"
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

func joinErrors(errs []error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	msg := make([]string, 0, len(filtered))
	for _, err := range filtered {
		msg = append(msg, err.Error())
	}
	return fmt.Errorf("%s", strings.Join(msg, "; "))
}
