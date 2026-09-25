package mailer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// withDataFile points the package-level dataFile at a temporary location for
// the duration of a test and restores it afterwards.
func withDataFile(t *testing.T, contents string) {
	t.Helper()
	original := dataFile
	t.Cleanup(func() { dataFile = original })
	dataFile = filepath.Join(t.TempDir(), "mass-mailer.json")
	if contents != "" {
		if err := os.WriteFile(dataFile, []byte(contents), 0600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
}

func TestSeedSettingsFromEnv(t *testing.T) {
	saved := `{"settings":{"host":"smtp.saved.example","port":587,"username":"saved","password":"pw","encryption":"tls"},"batches":null}`

	tests := []struct {
		name     string
		fixture  string
		env      map[string]string
		wantHost string
		wantPort int
		wantEnc  string
	}{
		{
			name:     "seeds when no data file exists",
			env:      map[string]string{"SMTP_HOST": "mailpit", "SMTP_PORT": "1025", "SMTP_ENCRYPTION": "none"},
			wantHost: "mailpit",
			wantPort: 1025,
			wantEnc:  "none",
		},
		{
			name:     "saved settings win over the environment",
			fixture:  saved,
			env:      map[string]string{"SMTP_HOST": "mailpit", "SMTP_PORT": "1025", "SMTP_ENCRYPTION": "none"},
			wantHost: "smtp.saved.example",
			wantPort: 587,
			wantEnc:  "tls",
		},
		{
			name:    "no environment leaves settings empty",
			wantEnc: "",
		},
		{
			name:     "encryption defaults to STARTTLS when unset",
			env:      map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_PORT": "587"},
			wantHost: "smtp.example.com",
			wantPort: 587,
			wantEnc:  "tls",
		},
		{
			name:     "non-numeric port is ignored rather than fatal",
			env:      map[string]string{"SMTP_HOST": "mailpit", "SMTP_PORT": "not-a-number"},
			wantHost: "mailpit",
			wantPort: 0,
			wantEnc:  "tls",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withDataFile(t, tc.fixture)
			for _, key := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_ENCRYPTION"} {
				t.Setenv(key, tc.env[key])
			}

			app := &App{}
			if err := app.load(); err != nil {
				t.Fatalf("load: %v", err)
			}
			got := app.store.Settings
			if got.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", got.Host, tc.wantHost)
			}
			if got.Port != tc.wantPort {
				t.Errorf("port = %d, want %d", got.Port, tc.wantPort)
			}
			if got.Encryption != tc.wantEnc {
				t.Errorf("encryption = %q, want %q", got.Encryption, tc.wantEnc)
			}
		})
	}
}

// Seeding must not persist: the environment supplies defaults on every boot,
// so clearing the data file returns to them rather than to a stale snapshot.
func TestSeedingDoesNotWriteToDisk(t *testing.T) {
	withDataFile(t, "")
	t.Setenv("SMTP_HOST", "mailpit")
	t.Setenv("SMTP_PORT", "1025")

	app := &App{}
	if err := app.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := os.Stat(dataFile); !os.IsNotExist(err) {
		t.Fatalf("load() created %s; seeding must stay in memory", dataFile)
	}
}

// A save from the Settings page must take precedence on the next boot.
func TestSavedSettingsOverrideEnvOnReload(t *testing.T) {
	withDataFile(t, "")
	t.Setenv("SMTP_HOST", "mailpit")
	t.Setenv("SMTP_PORT", "1025")

	app := &App{}
	if err := app.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	app.store.Settings = Settings{Host: "smtp.real.example", Port: 587, Encryption: "tls"}
	if err := app.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded := &App{}
	if err := reloaded.load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.store.Settings.Host != "smtp.real.example" {
		t.Errorf("host = %q, want the saved value to win over SMTP_HOST", reloaded.store.Settings.Host)
	}
}

func TestLoadRejectsCorruptDataFile(t *testing.T) {
	withDataFile(t, "{not json")
	app := &App{}
	if err := app.load(); err == nil {
		t.Fatal("load() accepted a corrupt data file; want an error")
	}
}

func TestLoadPreservesBatches(t *testing.T) {
	withDataFile(t, `{"settings":{"host":"","port":0},"batches":[{"id":"abc","name":"Keep me"}]}`)
	t.Setenv("SMTP_HOST", "mailpit")

	app := &App{}
	if err := app.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(app.store.Batches) != 1 || app.store.Batches[0].Name != "Keep me" {
		out, _ := json.Marshal(app.store.Batches)
		t.Fatalf("batches were lost during seeding: %s", out)
	}
	if app.store.Settings.Host != "mailpit" {
		t.Errorf("host = %q, want seeding to still apply when only batches were saved", app.store.Settings.Host)
	}
}
