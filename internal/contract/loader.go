package contract

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

func LoadFile(path string) (ServiceContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ServiceContract{}, fmt.Errorf("read service contract %q: %w", path, err)
	}
	contract, err := Decode(data)
	if err != nil {
		return ServiceContract{}, fmt.Errorf("decode service contract %q: %w", path, err)
	}
	contract.Source = path
	return contract, nil
}

func Decode(data []byte) (ServiceContract, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var contract ServiceContract
	if err := decoder.Decode(&contract); err != nil {
		return ServiceContract{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return ServiceContract{}, errors.New("multiple YAML documents are not allowed")
		}
		return ServiceContract{}, err
	}
	if err := Validate(contract); err != nil {
		return ServiceContract{}, err
	}
	hash, err := Hash(contract)
	if err != nil {
		return ServiceContract{}, err
	}
	contract.ContentHash = hash
	return contract, nil
}

func LoadDir(path string) ([]ServiceContract, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read contract directory %q: %w", path, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension == ".yaml" || extension == ".yml" || extension == ".json" {
			paths = append(paths, filepath.Join(path, entry.Name()))
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("contract directory %q contains no YAML or JSON contracts", path)
	}
	contracts := make([]ServiceContract, 0, len(paths))
	for _, path := range paths {
		contract, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		contracts = append(contracts, contract)
	}
	return contracts, nil
}
