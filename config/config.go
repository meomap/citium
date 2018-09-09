package config

import (
	"os"

	"github.com/pkg/errors"
)

// Configuration defines runtime variables
type Configuration struct {
	TableName string `json:"table_name"`
	BaseURL   string `json:"base_url"`
	Token     string `json:"api_token"`
	UserAgent string `json:"user_agent"`
}

// NewConfiguration returns config initialized from environment variables
func NewConfiguration() (*Configuration, error) {
	table := os.Getenv("TABLE_NAME")
	if table == "" {
		return nil, errors.New("Require environment variable TABLE_NAME")
	}
	return &Configuration{
		TableName: table,
		BaseURL:   os.Getenv("BASE_URL"),
		Token:     os.Getenv("API_TOKEN"),
		UserAgent: os.Getenv("USER_AGENT"),
	}, nil
}

// Must ensures configuration is properly initialized
func Must(conf *Configuration, err error) *Configuration {
	if err != nil {
		panic(err)
	}
	return conf
}
