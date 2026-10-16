package skills

import (
	"strings"
	"testing"
)

func TestBuiltinPackagesUseKAIBrandAndKeepStableWorkflowIdentity(t *testing.T) {
	packages, err := loadBuiltinSkillPackages(nil)
	if err != nil {
		t.Fatal(err)
	}
	foundWorkflow := false
	for _, item := range packages {
		if item.skill.ID == "yingce-short-drama-workflow" {
			foundWorkflow = true
			if item.skill.Name != "KAI影短剧制作工作流" || item.skill.OwnerID != "yingce-system" || item.skill.AuthorName != "影策" {
				t.Fatalf("workflow branding changed its stable identity or attribution: %#v", item.skill)
			}
		}
		for path, content := range item.archive.Files {
			if strings.Contains(path, "LICENSE") {
				continue
			}
			for index, line := range strings.Split(string(content), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "author:") {
					continue
				}
				if strings.Contains(line, "影策") || strings.Contains(line, "北斗") {
					t.Errorf("upstream product copy in %s/%s:%d", item.skill.ID, path, index+1)
				}
			}
		}
	}
	if !foundWorkflow {
		t.Fatal("stable short-drama workflow ID disappeared")
	}
}
