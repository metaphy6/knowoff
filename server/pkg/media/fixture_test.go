package media

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHistoricalImageFixturesRemainUnreadableByTextRuntime(t *testing.T) {
	for _, name := range []string{"golden-pack", "band-starved-pack"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join("testdata", name)
			before := retirementTreeHashes(t, root)
			if _, err := LoadTextPack(root, textFixtureLimits()); err == nil {
				t.Fatal("legacy format1 fixture admitted")
			}
			after := retirementTreeHashes(t, root)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("legacy evidence bytes changed during refusal")
			}
		})
	}
}

func TestCurrentTextFixtureCertifiesEngineeringOnly(t *testing.T) {
	snapshot, err := LoadTextPack("testdata/text-en", textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	report := CertifyText(snapshot, textFixtureTuning(), 20, 71)
	if !report.Passed || report.ProductionReady || len(report.Cells) != 10 {
		t.Fatal("text fixture certification scope changed", report)
	}
	if err := snapshot.ValidateActivation("text-v1", textFixtureTuning()); err == nil {
		t.Fatal("synthetic evidence enabled production")
	}
}

func TestPlayableImageExecutableSurfaceRetired(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"Pack": true, "MediaItem": true, "CardItem": true, "SignedURLIssuer": true, "LoadPack": true, "NewDealer": true, "NewSignedURLIssuer": true, "BuildCandidates": true, "Cosine": true}
	for _, pkg := range packages {
		for name, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				var symbol string
				switch node := node.(type) {
				case *ast.TypeSpec:
					symbol = node.Name.Name
				case *ast.FuncDecl:
					symbol = node.Name.Name
				}
				if forbidden[symbol] {
					t.Errorf("retired playable-image executable %s remains in %s", symbol, name)
				}
				return true
			})
		}
	}
}
