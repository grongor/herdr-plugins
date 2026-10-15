package plugin

import "testing"

func TestVersionReadsTheManifest(t *testing.T) {
	got := Version([]byte("id = \"grongor.example\"\nversion = \"1.2.3\"\n\n[[actions]]\nid = \"open\"\n"))
	if got != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", got)
	}
}

func TestVersionPanicsWithoutOne(t *testing.T) {
	for name, manifest := range map[string]string{
		"missing": "id = \"grongor.example\"\n",
		"invalid": "version = \n",
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("Version did not panic")
				}
			}()
			Version([]byte(manifest))
		})
	}
}
