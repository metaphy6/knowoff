// mediapack is the media-pipeline CLI for Knowoff.
//
// Subcommands:
//
//	build      - generate a deterministic synthetic pack (ingest/screen/tag/embed/bundle)
//	certify    - run the certification gate on a bundle directory
//	simulate   - run Monte Carlo deal feasibility and print a report
//	publish    - copy a bundle to an output directory
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/knowoff/knowoff/tools/mediapack/internal/bundlewriter"
	"github.com/knowoff/knowoff/tools/mediapack/internal/generator"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	switch cmd {
	case "text-prepare", "text-build-fixture", "text-certify", "text-simulate", "text-publish", "text-duplicates":
		os.Exit(runTextCommand(cmd, os.Args[2:], os.Stdout, os.Stderr))
	case "build":
		os.Exit(cmdBuild(os.Args[2:]))
	case "certify":
		os.Exit(cmdCertify(os.Args[2:]))
	case "simulate":
		os.Exit(cmdSimulate(os.Args[2:]))
	case "publish":
		os.Exit(cmdPublish(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mediapack <build|certify|simulate|publish|text-prepare|text-build-fixture|text-certify|text-simulate|text-publish|text-duplicates> [options]")
}

func cmdBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	out := fs.String("out", "", "output bundle directory")
	tag := fs.String("tag", "", "pack tag (defaults to synthetic-<date>)")
	nowns := fs.Int("nowns", 10, "number of Nowns")
	cardsPerNown := fs.Int("cards-per-nown", 20, "cards per Nown")
	seed := fs.Int64("seed", 42, "deterministic seed")
	bandStarved := fs.Bool("band-starved", false, "generate the band-starved negative fixture")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "build requires -out")
		return 2
	}

	var pack *media.Pack
	var err error
	if *bandStarved {
		pack, err = generator.BandStarvedPack(*seed)
	} else {
		pack, err = generator.SyntheticPack(*tag, *nowns, *cardsPerNown, *seed)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate: %v\n", err)
		return 1
	}
	if err := bundlewriter.Write(pack, *out); err != nil {
		fmt.Fprintf(os.Stderr, "write bundle: %v\n", err)
		return 1
	}
	fmt.Println(*out)
	return 0
}

func cmdCertify(args []string) int {
	fs := flag.NewFlagSet("certify", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "certify requires a bundle directory")
		return 2
	}
	pack, err := media.LoadPack(fs.Arg(0), defaultDealing())
	if err != nil {
		fmt.Fprintf(os.Stderr, "load pack: %v\n", err)
		return 1
	}
	cert := media.Certify(pack, defaultDealing(), defaultHand())
	out, _ := json.MarshalIndent(cert, "", "  ")
	fmt.Println(string(out))
	if !cert.Passed {
		return 1
	}
	return 0
}

func cmdSimulate(args []string) int {
	fs := flag.NewFlagSet("simulate", flag.ExitOnError)
	players := fs.Int("players", 4, "table size (4 or 6)")
	rounds := fs.Int("rounds", 100, "number of Monte Carlo rounds")
	seed := fs.Int64("seed", 1, "deterministic seed")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "simulate requires a bundle directory")
		return 2
	}
	if *players != 4 && *players != 6 {
		fmt.Fprintln(os.Stderr, "--players must be 4 or 6")
		return 2
	}
	pack, err := media.LoadPack(fs.Arg(0), defaultDealing())
	if err != nil {
		fmt.Fprintf(os.Stderr, "load pack: %v\n", err)
		return 1
	}

	dealer := &media.Dealer{Pack: pack, Dealing: defaultDealing(), Hand: defaultHand()}
	nowns := nownSchedule(pack, *players)
	success, total, err := media.DealFeasible(dealer, *players, nowns, *rounds, *seed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulate: %v\n", err)
		return 1
	}

	rng := rand.New(rand.NewSource(*seed))
	report := map[string]any{
		"pack_tag":         pack.Manifest.PackTag,
		"players":          *players,
		"rounds":           total,
		"success":          success,
		"feasible":         success == total,
		"seed":             *seed,
		"sample_hand_hash": rng.Int63(), // deterministic consumption for byte stability
	}
	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
	if success != total {
		return 1
	}
	return 0
}

func cmdPublish(args []string) int {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	out := fs.String("out", "", "output directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "publish requires a bundle directory and -out")
		return 2
	}
	src := fs.Arg(0)
	if err := copyDir(src, *out); err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n", err)
		return 1
	}
	fmt.Println(*out)
	return 0
}

func defaultDealing() media.DealingTuning {
	return media.DealingTuning{BandHigh: 0.55, BandLow: 0.30, MinHighPerNown: 2, MinDistantPerNown: 2}
}

func defaultHand() media.HandTuning {
	return media.HandTuning{Size: 5, DrawPile: 3}
}

func nownSchedule(pack *media.Pack, size int) []string {
	rounds := 2
	if size == 6 {
		rounds = 3
	}
	if rounds > len(pack.Media) {
		rounds = len(pack.Media)
	}
	var ids []string
	for i := 0; i < rounds; i++ {
		ids = append(ids, pack.Media[i].ID)
	}
	return ids
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}
