package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/knowoff/knowoff/server/pkg/textcert"
	"github.com/knowoff/knowoff/tools/mediapack/internal/generator"
	"gopkg.in/yaml.v3"
)

// The text commands share the runtime schema/dealer/certifier. They prepare
// files only; authenticated activation/audit and contribution rewards remain
// server operations. No command fabricates human/screening/action gate records.
func runTextCommand(command string, args []string, out, errout io.Writer) int {
	if command != "text-prepare" && command != "text-build-fixture" && command != "text-certify" && command != "text-simulate" && command != "text-publish" && command != "text-duplicates" && command != "text-actions" {
		fmt.Fprintln(errout, "unknown text command")
		return 2
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errout)
	tuningPath := fs.String("tuning", "", "required gameplay tuning YAML")
	output := fs.String("out", "", "new output directory or report file")
	input := fs.String("input", "", "accepted immutable TextBundle JSON")
	language := fs.String("language", "", "synthetic fixture language")
	rules := fs.String("rules", "", "compatible rules version")
	release := fs.String("release", "", "immutable fixture release ID")
	samples := fs.Int("samples", 0, "explicit sampled schedules per mode/size")
	seed := fs.Int64("seed", 0, "explicit privileged simulation seed")
	replayPath := fs.String("replay-out", "", "new private replay input file")
	evidenceDir := fs.String("evidence-dir", "", "reviewed JSON gate artifacts to attach during preparation")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *tuningPath == "" {
		fmt.Fprintln(errout, "text commands require -tuning")
		return 2
	}
	limits, tuning, err := loadTextTuning(*tuningPath)
	if err != nil {
		fmt.Fprintln(errout, "tuning:", err)
		return 1
	}
	fail := func(err error) int { fmt.Fprintln(errout, command+":", err); return 1 }
	if command == "text-duplicates" {
		if *input == "" || *output == "" || fs.NArg() != 0 {
			fmt.Fprintln(errout, "duplicate inspection requires -input and -out")
			return 2
		}
		raw, err := readTextInput(*input, limits.MaxBundleBytes)
		if err != nil {
			return fail(err)
		}
		candidates, err := media.InspectTextDuplicates(raw, limits)
		if err != nil {
			return fail(err)
		}
		raw, err = json.Marshal(candidates)
		if err != nil {
			return fail(err)
		}
		if err := writeTextArtifact(*output, raw); err != nil {
			return fail(err)
		}
		fmt.Fprintf(out, "%d duplicate candidates require editorial decisions\n", len(candidates))
		return 0
	}
	if command == "text-build-fixture" || command == "text-prepare" {
		if fs.NArg() != 0 || *output == "" {
			fmt.Fprintln(errout, "preparation requires -out and no positional arguments")
			return 2
		}
		var bundle media.TextBundle
		if command == "text-build-fixture" {
			if *language == "" || *rules == "" || *release == "" {
				fmt.Fprintln(errout, "fixture requires -language, -rules and -release")
				return 2
			}
			bundle, err = generator.SyntheticTextBundle(*language, *rules, *release, limits)
		} else {
			if *input == "" {
				fmt.Fprintln(errout, "text-prepare requires -input")
				return 2
			}
			var raw []byte
			raw, err = readTextInput(*input, limits.MaxBundleBytes)
			if err == nil {
				bundle, err = media.DecodeTextBundle(raw, limits)
			}
		}
		if err != nil {
			return fail(err)
		}
		if *evidenceDir != "" {
			if command == "text-build-fixture" {
				return fail(fmt.Errorf("synthetic builder cannot attach human evidence"))
			}
			if bundle.Artifacts == nil {
				bundle.Artifacts = map[string][]byte{}
			}
			if bundle.Manifest.CertificationArtifacts == nil {
				bundle.Manifest.CertificationArtifacts = map[string]string{}
			}
			for _, name := range []string{"technical.json", "editorial.json", "actions.json", "screening.json", "release.json", "replay.json", "action-replay.json"} {
				raw, err := readTextInput(filepath.Join(*evidenceDir, name), limits.MaxFileBytes)
				if err != nil {
					return fail(err)
				}
				bundle.Artifacts[name] = raw
				bundle.Manifest.CertificationArtifacts[name] = media.ContentHash(raw)
			}
			bundle, err = media.SealTextBundle(bundle, limits)
			if err != nil {
				return fail(err)
			}
		}
		if err := media.WriteTextBundle(*output, bundle, limits); err != nil {
			return fail(err)
		}
		fmt.Fprintln(out, "prepared", *output)
		return 0
	}
	if fs.NArg() != 1 || *output == "" {
		fmt.Fprintln(errout, "text operation requires one bundle directory and -out")
		return 2
	}
	snapshot, err := media.LoadTextPack(fs.Arg(0), limits)
	if err != nil {
		return fail(err)
	}
	if command == "text-publish" {
		if *rules == "" {
			fmt.Fprintln(errout, "text-publish requires -rules")
			return 2
		}
		fullTuning, err := loadActionTuning(*tuningPath)
		if err != nil {
			return fail(err)
		}
		if err := textcert.ValidateActivation(snapshot, *rules, fullTuning); err != nil {
			return fail(err)
		}
		if _, err := os.Lstat(*output); err == nil {
			existing, err := media.LoadTextPack(*output, limits)
			if err != nil || existing.ManifestSHA256() != snapshot.ManifestSHA256() {
				return fail(fmt.Errorf("immutable publication destination conflict"))
			}
			fmt.Fprintln(out, "already published", *output)
			return 0
		} else if !os.IsNotExist(err) {
			return fail(err)
		}
		if err := media.WriteTextBundle(*output, snapshot.Bundle(), limits); err != nil {
			return fail(err)
		}
		fmt.Fprintln(out, "published", *output)
		return 0
	}
	seedSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "seed" {
			seedSet = true
		}
	})
	if command == "text-actions" {
		if *samples < 1 || !seedSet {
			return fail(fmt.Errorf("actions require positive -samples and explicit nonzero -seed"))
		}
		fullTuning, err := loadActionTuning(*tuningPath)
		if err != nil {
			return fail(err)
		}
		evidence, err := textcert.Certify(snapshot, fullTuning, *samples, *seed)
		if err != nil {
			return fail(err)
		}
		raw, err := json.Marshal(evidence)
		if err != nil {
			return fail(err)
		}
		if err := writeTextArtifact(*output, raw); err != nil {
			return fail(err)
		}
		fmt.Fprintln(out, "private action evidence written; sampled full schedules, human release gates remain required")
		return 0
	}
	if *samples < 1 || !seedSet || *replayPath == "" {
		fmt.Fprintln(errout, "simulation requires positive -samples, explicit -seed and -replay-out")
		return 2
	}
	report := media.CertifyText(snapshot, tuning, *samples, *seed)
	replay := media.NewTextReplay(snapshot, tuning, *samples, *seed)
	raw, err := json.Marshal(replay)
	if err != nil {
		return fail(err)
	}
	if err := writeTextArtifact(*replayPath, raw); err != nil {
		return fail(err)
	}
	raw, err = json.Marshal(report)
	if err != nil {
		return fail(err)
	}
	if err := writeTextArtifact(*output, raw); err != nil {
		return fail(err)
	}
	if _, err := fmt.Fprintln(out, string(raw)); err != nil {
		return fail(err)
	}
	if !report.Passed {
		return 1
	}
	return 0
}

func readTextInput(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes || maxBytes < 1 {
		return nil, fmt.Errorf("input is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("input byte bound exceeded")
	}
	return raw, nil
}

func writeTextArtifact(path string, raw []byte) (err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}()
	_, err = file.Write(raw)
	if err == nil {
		err = file.Sync()
	}
	return err
}

// These are projections of the explicitly supplied repository tuning document,
// not runtime defaults. Other gameplay sections are deliberately left to the
// server config loader; every numeric input to this tool comes from this file.
func loadTextTuning(path string) (media.TextLimits, media.TextDealTuning, error) {
	var config struct {
		Contract struct {
			MaxTextBytes int `yaml:"max_text_bytes"`
		} `yaml:"contract"`
		TextCatalog struct {
			MaxRecords     int   `yaml:"max_records"`
			MaxFileBytes   int64 `yaml:"max_file_bytes"`
			MaxBundleBytes int64 `yaml:"max_bundle_bytes"`
			MaxSearchNodes int   `yaml:"max_search_nodes"`
		} `yaml:"text_catalog"`
		Hand struct {
			Size     int `yaml:"size"`
			DrawPile int `yaml:"draw_pile"`
		} `yaml:"hand"`
		Dealing struct {
			MinHigh    int `yaml:"min_high_per_nown"`
			MinDistant int `yaml:"min_distant_per_nown"`
		} `yaml:"dealing"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return media.TextLimits{}, media.TextDealTuning{}, err
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return media.TextLimits{}, media.TextDealTuning{}, err
	}
	limits := media.TextLimits{MaxTextBytes: config.Contract.MaxTextBytes, MaxRecords: config.TextCatalog.MaxRecords, MaxFileBytes: config.TextCatalog.MaxFileBytes, MaxBundleBytes: config.TextCatalog.MaxBundleBytes}
	tuning := media.TextDealTuning{HandSize: config.Hand.Size, ReserveSize: config.Hand.DrawPile, MinHigh: config.Dealing.MinHigh, MinDistant: config.Dealing.MinDistant, MaxSearchNodes: config.TextCatalog.MaxSearchNodes}
	if limits.MaxTextBytes < 1 || limits.MaxRecords < 1 || limits.MaxFileBytes < 1 || limits.MaxBundleBytes < limits.MaxFileBytes || tuning.HandSize < 1 || tuning.ReserveSize < 0 || tuning.MinHigh < 1 || tuning.MinDistant < 1 || tuning.MinHigh+tuning.MinDistant > tuning.HandSize || tuning.MaxSearchNodes < 1 {
		return media.TextLimits{}, media.TextDealTuning{}, fmt.Errorf("missing or invalid text tuning values")
	}
	return limits, tuning, nil
}

func loadActionTuning(path string) (textcert.Tuning, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return textcert.Tuning{}, err
	}
	return textcert.DecodeTuning(raw)
}
