package config

import (
	"os"

	"github.com/pkg/errors"
)

// Configuration defines runtime variables
type Configuration struct {
	TableName string `json:"table_name"`
	// Region    string `json:"region"`
	BaseURL   string `json:"base_url"`
	Token     string `json:"api_token"`
	UserAgent string `json:"user_agent"`
}

// NewConfiguration returns config initalized from environment variables
func NewConfiguration() (*Configuration, error) {
	table := os.Getenv("TABLE_NAME")
	if table == "" {
		return nil, errors.New("Require environment variable TABLE_NAME")
	}
	// region := os.Getenv("AWS_REGION")
	// if region == "" {
	// 	return nil, errors.New("Require environment variable AWS_REGION")
	// }
	return &Configuration{
		TableName: table,
		// Region:    region,
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
