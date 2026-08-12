package catalog

import (
	"encoding/json"
	"fmt"
	"os"

	"agentwritegateway/internal/domain"
)

func Load(path string) ([]domain.Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read service catalog: %w", err)
	}
	var services []domain.Service
	if err := json.Unmarshal(data, &services); err != nil {
		return nil, fmt.Errorf("decode service catalog: %w", err)
	}
	return services, nil
}
