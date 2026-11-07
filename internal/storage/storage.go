package storage

import (
	"encoding/json"
	"gemini-switcher/internal/model"
	"os"
	"path/filepath"
	"sync"
)

type Storage struct {
	dir string
	mu  sync.RWMutex
}

func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Storage{dir: dir}, nil
}

func (s *Storage) configPath() string {
	return filepath.Join(s.dir, "config.json")
}

func (s *Storage) AccountsFilePath() string {
	return filepath.Join(s.dir, "accounts.json")
}

func (s *Storage) accountsPath() string {
	return s.AccountsFilePath()
}

func (s *Storage) LoadConfig() (*model.Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.configPath())
	if os.IsNotExist(err) {
		return model.DefaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}

	var cfg model.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return model.DefaultConfig(), nil
	}
	return &cfg, nil
}

func (s *Storage) SaveConfig(cfg *model.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath(), data, 0600)
}

func (s *Storage) LoadAccounts() ([]*model.AccountProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p := s.accountsPath()
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return []*model.AccountProfile{}, nil
	}
	if err != nil {
		return nil, err
	}

	var accs []*model.AccountProfile
	if err := json.Unmarshal(data, &accs); err != nil {
		if bakData, bakErr := os.ReadFile(p + ".bak"); bakErr == nil {
			var bakAccs []*model.AccountProfile
			if bakErr := json.Unmarshal(bakData, &bakAccs); bakErr == nil {
				return bakAccs, nil
			}
		}
		return nil, err
	}
	return accs, nil
}

func (s *Storage) SaveAccounts(accs []*model.AccountProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := s.accountsPath()
	if existing, err := os.ReadFile(p); err == nil && len(existing) > 0 {
		_ = os.WriteFile(p+".bak", existing, 0600)
	}

	data, err := json.MarshalIndent(accs, "", "  ")
	if err != nil {
		return err
	}
	if _, err := os.Stat(p + ".bak"); os.IsNotExist(err) {
		_ = os.WriteFile(p+".bak", data, 0600)
	}
	return os.WriteFile(p, data, 0600)
}
