package cmd

import (
	"reflect"
	"testing"
)

func TestParseDocumentParameters(t *testing.T) {
	tests := []struct {
		name string
		args []string
		base map[string][]string
		want map[string][]string
	}{
		{
			name: "no args returns base unchanged",
			args: nil,
			base: map[string][]string{"linuxcmd": {"top"}},
			want: map[string][]string{"linuxcmd": {"top"}},
		},
		{
			name: "single key=value",
			args: []string{"linuxcmd=top"},
			want: map[string][]string{"linuxcmd": {"top"}},
		},
		{
			name: "distinct keys",
			args: []string{"linuxcmd=top", "runAsUser=ec2-user"},
			want: map[string][]string{"linuxcmd": {"top"}, "runAsUser": {"ec2-user"}},
		},
		{
			name: "repeated key accumulates in order",
			args: []string{"cmd=uptime", "cmd=whoami"},
			want: map[string][]string{"cmd": {"uptime", "whoami"}},
		},
		{
			name: "value may contain equals signs",
			args: []string{`cmd=export FOO=bar && echo $FOO`},
			want: map[string][]string{"cmd": {"export FOO=bar && echo $FOO"}},
		},
		{
			name: "empty value is allowed",
			args: []string{"cmd="},
			want: map[string][]string{"cmd": {""}},
		},
		{
			name: "cli values append to config file values",
			args: []string{"cmd=whoami"},
			base: map[string][]string{"cmd": {"uptime"}},
			want: map[string][]string{"cmd": {"uptime", "whoami"}},
		},
		{
			name: "cli adds a key alongside config file keys",
			args: []string{"runAsUser=ec2-user"},
			base: map[string][]string{"linuxcmd": {"top"}},
			want: map[string][]string{"linuxcmd": {"top"}, "runAsUser": {"ec2-user"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDocumentParameters(tt.args, tt.base)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseDocumentParameters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseDocumentParameters_Invalid(t *testing.T) {
	for _, arg := range []string{"linuxcmd", "", "=novalue"} {
		t.Run(arg, func(t *testing.T) {
			if _, err := parseDocumentParameters([]string{arg}, nil); err == nil {
				t.Errorf("parseDocumentParameters(%q) expected an error, got nil", arg)
			}
		})
	}
}

// The base map belongs to the shared config singleton, so it must not be mutated in place.
func TestParseDocumentParameters_DoesNotMutateBase(t *testing.T) {
	base := map[string][]string{"cmd": {"uptime"}}

	if _, err := parseDocumentParameters([]string{"cmd=whoami"}, base); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := []string{"uptime"}; !reflect.DeepEqual(base["cmd"], want) {
		t.Errorf("base was mutated: base[cmd] = %v, want %v", base["cmd"], want)
	}
}
