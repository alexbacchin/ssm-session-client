package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSSHConfig_BasicHost(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	content := `Host my-ec2
  HostName i-0123456789abcdef0
  User ec2-user
  Port 2222
  IdentityFile ~/.ssh/my-key
`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-ec2")

	if cfg.HostName != "i-0123456789abcdef0" {
		t.Errorf("expected HostName=i-0123456789abcdef0, got %q", cfg.HostName)
	}
	if cfg.User != "ec2-user" {
		t.Errorf("expected User=ec2-user, got %q", cfg.User)
	}
	if cfg.Port != "2222" {
		t.Errorf("expected Port=2222, got %q", cfg.Port)
	}
}

func TestParseSSHConfig_WildcardPattern(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	content := `Host i-*
  User ec2-user
  StrictHostKeyChecking no

Host *
  ServerAliveInterval 60
`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "i-0123456789abcdef0")

	if cfg.User != "ec2-user" {
		t.Errorf("expected User=ec2-user, got %q", cfg.User)
	}
	if cfg.StrictHostKeyCheck != "no" {
		t.Errorf("expected StrictHostKeyChecking=no, got %q", cfg.StrictHostKeyCheck)
	}
	if cfg.ServerAliveInterval != "60" {
		t.Errorf("expected ServerAliveInterval=60, got %q", cfg.ServerAliveInterval)
	}
}

func TestParseSSHConfig_FirstMatchWins(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	content := `Host my-ec2
  User admin

Host my-*
  User default-user
  Port 2222
`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-ec2")

	// "admin" from first match should win over "default-user"
	if cfg.User != "admin" {
		t.Errorf("expected User=admin (first match wins), got %q", cfg.User)
	}
	// Port only appears in second block, so it should be picked up
	if cfg.Port != "2222" {
		t.Errorf("expected Port=2222, got %q", cfg.Port)
	}
}

func TestParseSSHConfig_NoMatch(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	content := `Host other-host
  User admin
`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-host")

	if cfg.User != "" {
		t.Errorf("expected empty User for non-matching host, got %q", cfg.User)
	}
}

func TestParseSSHConfig_NonexistentFile(t *testing.T) {
	cfg := ParseSSHConfig("/nonexistent/config", "myhost")
	if cfg.HostName != "" {
		t.Errorf("expected empty config for nonexistent file, got HostName=%q", cfg.HostName)
	}
}

func TestParseSSHConfig_Comments(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	content := `# This is a comment
Host my-ec2
  # Another comment
  HostName i-abc123
  User ec2-user
`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-ec2")

	if cfg.HostName != "i-abc123" {
		t.Errorf("expected HostName=i-abc123, got %q", cfg.HostName)
	}
}

func TestParseSSHConfig_EqualsFormat(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	content := `Host my-ec2
  HostName=i-abc123
  User=ec2-user
`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-ec2")

	if cfg.HostName != "i-abc123" {
		t.Errorf("expected HostName=i-abc123, got %q", cfg.HostName)
	}
	if cfg.User != "ec2-user" {
		t.Errorf("expected User=ec2-user, got %q", cfg.User)
	}
}

func TestParseSSHConfig_WindowsCRLF(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	// Simulate a file saved on Windows with CRLF line endings.
	content := "Host my-ec2\r\n  HostName i-0123456789abcdef0\r\n  User ec2-user\r\n"
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-ec2")

	if cfg.HostName != "i-0123456789abcdef0" {
		t.Errorf("expected HostName=i-0123456789abcdef0, got %q", cfg.HostName)
	}
	if cfg.User != "ec2-user" {
		t.Errorf("expected User=ec2-user, got %q", cfg.User)
	}
}

func TestParseSSHConfig_WindowsBOM(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	// Simulate a file with a UTF-8 BOM — added by Notepad and some Windows editors.
	const bom = "\xef\xbb\xbf"
	content := bom + "Host my-ec2\n  HostName i-0123456789abcdef0\n  User ec2-user\n"
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-ec2")

	if cfg.HostName != "i-0123456789abcdef0" {
		t.Errorf("expected HostName=i-0123456789abcdef0, got %q (BOM not stripped?)", cfg.HostName)
	}
	if cfg.User != "ec2-user" {
		t.Errorf("expected User=ec2-user, got %q", cfg.User)
	}
}

func TestParseSSHConfig_WindowsBOMAndCRLF(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	// Simulate a file with both UTF-8 BOM and CRLF line endings.
	const bom = "\xef\xbb\xbf"
	content := bom + "Host my-ec2\r\n  HostName i-0123456789abcdef0\r\n  User ec2-user\r\n"
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-ec2")

	if cfg.HostName != "i-0123456789abcdef0" {
		t.Errorf("expected HostName=i-0123456789abcdef0, got %q (BOM+CRLF not handled?)", cfg.HostName)
	}
	if cfg.User != "ec2-user" {
		t.Errorf("expected User=ec2-user, got %q", cfg.User)
	}
}

// utf16LE encodes an ASCII string as UTF-16 LE bytes with BOM, mimicking
// PowerShell 5.x Out-File / ">" redirect default output.
func utf16LE(s string) []byte {
	out := []byte{0xFF, 0xFE} // BOM
	for i := 0; i < len(s); i++ {
		out = append(out, s[i], 0x00)
	}
	return out
}

func TestParseSSHConfig_PowerShellUTF16LE(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	// PowerShell 5.x Out-File and ">" write UTF-16 LE with BOM by default.
	content := "Host my-ec2\r\n  HostName i-0123456789abcdef0\r\n  User ec2-user\r\n"
	if err := os.WriteFile(configFile, utf16LE(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSSHConfig(configFile, "my-ec2")

	if cfg.HostName != "i-0123456789abcdef0" {
		t.Errorf("expected HostName=i-0123456789abcdef0, got %q (UTF-16 LE not handled?)", cfg.HostName)
	}
	if cfg.User != "ec2-user" {
		t.Errorf("expected User=ec2-user, got %q", cfg.User)
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		s, pattern string
		want       bool
	}{
		{"i-abc123", "i-*", true},
		{"i-abc123", "i-abc123", true},
		{"i-abc123", "i-abc???", true},
		{"i-abc123", "j-*", false},
		{"myhost", "*", true},
		{"", "*", true},
		{"", "?", false},
		{"a", "?", true},
	}

	for _, tt := range tests {
		got := matchGlob(tt.s, tt.pattern)
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.s, tt.pattern, got, tt.want)
		}
	}
}
