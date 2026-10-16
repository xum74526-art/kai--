package hostupdate

import (
	"path/filepath"
	"testing"
)

func TestRepositoryDefaultsToKAIAndPreservesExplicitOverride(t *testing.T) {
	for _, repository := range []string{"", "example/custom-canvas"} {
		t.Run(repository, func(t *testing.T) {
			root := t.TempDir()
			manager, err := NewManager(Config{
				Repository: repository,
				InstallDir: root,
				StateDir:   filepath.Join(root, "state"),
				BackupDir:  filepath.Join(root, "backups"),
			})
			if err != nil {
				t.Fatal(err)
			}
			want := repository
			if want == "" {
				want = "xum74526-art/kai--"
			}
			if got := manager.Snapshot().Repository; got != want {
				t.Fatalf("repository = %q, want %q", got, want)
			}
		})
	}
}
