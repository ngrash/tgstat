// Package tgexport provides a way to read Telegram's result.json file.
package tgexport

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Result represents the result.json file.
type Result struct {
	Messages []Message `json:"messages"`
}

type Sender string

type Message struct {
	From         Sender       `json:"from"`
	TextEntities []TextEntity `json:"text_entities"`
	Date         Time         `json:"date"`
}

type TextEntity struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Time time.Time

func (t *Time) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return err
	}
	*t = Time(parsed)
	return nil
}

func ReadFile(path string) (*Result, error) {
	r, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	var data Result
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &data, nil
}
