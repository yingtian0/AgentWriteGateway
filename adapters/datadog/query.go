package datadog

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"themisy/pkg/adapter"
)

const (
	AdapterName    = "datadog"
	AdapterVersion = "datadog-metrics-v1/v1"
)

type Site string

const (
	SiteUS1    Site = "datadoghq.com"
	SiteUS3    Site = "us3.datadoghq.com"
	SiteUS5    Site = "us5.datadoghq.com"
	SiteEU1    Site = "datadoghq.eu"
	SiteAP1    Site = "ap1.datadoghq.com"
	SiteAP2    Site = "ap2.datadoghq.com"
	SiteUS1Fed Site = "ddog-gov.com"
	SiteUS2Fed Site = "us2.ddog-gov.com"
)

func (s Site) apiURL() (string, error) {
	switch s {
	case SiteUS1, SiteUS3, SiteUS5, SiteEU1, SiteAP1, SiteAP2, SiteUS1Fed, SiteUS2Fed:
		return "https://api." + string(s), nil
	default:
		return "", fmt.Errorf("unsupported Datadog site %q", s)
	}
}

type QueryKey struct {
	Service     string
	Environment string
}

// Query is trusted Runner configuration. The metrics expression is selected
// from this catalog and is never supplied by an agent or Action Grant.
type Query struct {
	Expression    string
	Comparator    string
	Threshold     float64
	Aggregation   string
	MinimumPoints int
	MaximumAge    time.Duration
}

type Config struct {
	Site    Site
	Queries map[QueryKey]Query
}

func (c Config) Validate() error {
	if _, err := c.Site.apiURL(); err != nil {
		return err
	}
	if len(c.Queries) == 0 {
		return errors.New("at least one Datadog query is required")
	}
	for key, query := range c.Queries {
		if strings.TrimSpace(key.Service) == "" || strings.TrimSpace(key.Environment) == "" || strings.TrimSpace(query.Expression) == "" {
			return errors.New("Datadog query target and expression are required")
		}
		switch query.Comparator {
		case "lte", "gte":
		default:
			return fmt.Errorf("unsupported comparator %q", query.Comparator)
		}
		switch query.Aggregation {
		case "last", "avg", "max":
		default:
			return fmt.Errorf("unsupported aggregation %q", query.Aggregation)
		}
		if query.MinimumPoints <= 0 || query.MaximumAge <= 0 {
			return fmt.Errorf("minimum points and maximum age must be positive for %s/%s", key.Service, key.Environment)
		}
	}
	return nil
}

func compare(value float64, query Query) adapter.VerificationStatus {
	if (query.Comparator == "lte" && value <= query.Threshold) || (query.Comparator == "gte" && value >= query.Threshold) {
		return adapter.VerificationPass
	}
	return adapter.VerificationFail
}
