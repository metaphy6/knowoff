package main

import (
	"encoding/json"
	"io"
	"runtime/debug"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/migrations"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

// No environment/config values or content text enter this build artifact.
func writeReleaseManifest(output io.Writer) error {
	schema, err := migrations.Compiled()
	if err != nil {
		return err
	}
	result := struct {
		Format      int                   `json:"format_version"`
		Protocol    int                   `json:"protocol_version"`
		Modes       []gamecontract.ModeID `json:"supported_modes"`
		Database    migrations.Manifest   `json:"database"`
		GoVersion   string                `json:"go_version,omitempty"`
		VCSRevision string                `json:"vcs_revision,omitempty"`
		VCSModified string                `json:"vcs_modified,omitempty"`
	}{Format: 1, Protocol: v2.Version, Modes: gamecontract.AllModes(), Database: schema}
	if build, ok := debug.ReadBuildInfo(); ok {
		result.GoVersion = build.GoVersion
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				result.VCSRevision = setting.Value
			case "vcs.modified":
				result.VCSModified = setting.Value
			}
		}
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
