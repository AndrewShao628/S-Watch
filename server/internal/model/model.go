// Package model loads the models trained by the Python pipeline in ml/ and runs
// inference in-process. No external AI service is involved at request time.
package model

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed artifacts/*.json
var artifacts embed.FS

const (
	recommenderArtifact = "recommender.json"
	classifierArtifact  = "review_classifier.json"
)

// Engine bundles the trained models the API serves.
type Engine struct {
	Recommender *Recommender
	Classifier  *Classifier
}

// Info is the model metadata exposed over the API.
type Info struct {
	Recommender ArtifactInfo `json:"recommender"`
	Classifier  ArtifactInfo `json:"classifier"`
}

// ArtifactInfo describes one trained artifact.
type ArtifactInfo struct {
	Kind      string         `json:"kind"`
	Version   string         `json:"version"`
	TrainedAt string         `json:"trained_at"`
	Dataset   map[string]any `json:"dataset,omitempty"`
	Metrics   map[string]any `json:"metrics,omitempty"`
}

// Load reads both artifacts. When dir is non-empty, artifacts are read from disk
// instead of the embedded copies, so a retrained model can be swapped in without
// rebuilding the binary.
func Load(dir string) (*Engine, error) {
	recommender := &Recommender{}
	if err := loadArtifact(dir, recommenderArtifact, recommender); err != nil {
		return nil, err
	}
	if err := recommender.prepare(); err != nil {
		return nil, fmt.Errorf("%s: %w", recommenderArtifact, err)
	}

	classifier := &Classifier{}
	if err := loadArtifact(dir, classifierArtifact, classifier); err != nil {
		return nil, err
	}
	if err := classifier.prepare(); err != nil {
		return nil, fmt.Errorf("%s: %w", classifierArtifact, err)
	}

	return &Engine{Recommender: recommender, Classifier: classifier}, nil
}

func (e *Engine) Info() Info {
	return Info{Recommender: e.Recommender.ArtifactInfo, Classifier: e.Classifier.ArtifactInfo}
}

func loadArtifact(dir, name string, target any) error {
	var (
		raw []byte
		err error
	)
	if dir != "" {
		raw, err = os.ReadFile(filepath.Join(dir, name))
	} else {
		raw, err = fs.ReadFile(artifacts, "artifacts/"+name)
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}
