package media

import (
	"os"
	"path/filepath"
	"testing"
)

// Only test-owned directories are written. Historical fixture inventories are
// read before/after refusal so retirement cannot silently rewrite archived data.
func retirementTreeHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[name] = ContentHash(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) == 0 {
		t.Fatal("empty historical fixture inventory")
	}
	return result
}

func retirementTextDirectory(t *testing.T, bundle TextBundle) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "text-pack")
	if err := WriteTextBundle(root, bundle, textFixtureLimits()); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestTextLoaderConfinesRootAndMembers(t *testing.T) {
	for _, kind := range []string{"root", "member"} {
		t.Run(kind, func(t *testing.T) {
			root := retirementTextDirectory(t, textFixtureBundle(t, "en"))
			outside := filepath.Join(t.TempDir(), "outside")
			if kind == "root" {
				if err := os.Symlink(root, outside); err != nil {
					t.Fatal(err)
				}
				root = outside
			} else {
				if err := os.WriteFile(outside, []byte("private sentinel"), 0600); err != nil {
					t.Fatal(err)
				}
				member := filepath.Join(root, "cards.jsonl")
				if err := os.Remove(member); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, member); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := LoadTextPack(root, textFixtureLimits()); err == nil {
				t.Fatal("unconfined text path accepted")
			}
		})
	}
}
