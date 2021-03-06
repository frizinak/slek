package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
)

var (
	stderr = log.New(os.Stderr, "", 0)

	// ErrFirstRun indicates a config file was created.
	ErrFirstRun = errors.New("created config file")
)

// Config contains slek config information.
type Config struct {
	Token     string `json:"token"`
	EditorCmd string `json:"editor"`
	// TODO interface type switch, strconv.Atoi if not an int.
	NotificationTimeout int    `json:"notification_timeout"`
	TimeFormat          string `json:"time_format"`
}

func createConfig(path string) error {
	f, err := os.Create(path)
	if f != nil {
		defer f.Close()
	}

	if err != nil {
		return err
	}

	_, err = f.WriteString(`{
    "token":    "-",
    "editor":   "",
	"notification_timeout": 2500,
	"time_format": "Jan 02 15:04:05"
}`)

	return err
}

func readConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if f != nil {
		defer f.Close()
	}

	if err != nil {
		return nil, err
	}

	c := &Config{}
	err = json.NewDecoder(f).Decode(c)

	return c, err
}

// Run parses the given config file if it exits.
// If not it will be created if create == true.
func Run(file string, create bool) (*Config, error) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		if create {
			stderr.Printf("creating default config file %s", file)
			if err := createConfig(file); err != nil {
				return nil, err
			}

			return nil, ErrFirstRun
		}

		return nil, fmt.Errorf("config file '%s' does not exist", file)
	}

	return readConfig(file)
}
